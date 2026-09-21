import {Fragment, useCallback, useEffect, useMemo, useState} from 'react';
import {Boxes, RotateCw, Settings} from 'lucide-react';
import {GetState, PickKubeconfig, RefreshPods, ResetKubeconfig} from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime";
import {kube, main} from "../wailsjs/go/models";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Separator} from "@/components/ui/separator";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Sidebar,
    SidebarContent,
    SidebarFooter,
    SidebarGroup,
    SidebarGroupContent,
    SidebarGroupLabel,
    SidebarHeader,
    SidebarInset,
    SidebarMenu,
    SidebarMenuButton,
    SidebarMenuItem,
    SidebarProvider,
    SidebarTrigger,
} from "@/components/ui/sidebar";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

const STATE_UPDATE_EVENT = "state:update";

const COLUMN_COUNT = 7;

// Radix Select treats an empty string as "no value", so the unfiltered choices need
// their own sentinel values.
const ALL_NAMESPACES = "__all__";
const NO_GROUPING = "__none__";

const NO_PODS: kube.PodInfo[] = [];

type View = "pod" | "settings";
type GroupBy = "none" | "namespace" | "node" | "status";

const GROUP_BY_LABELS: Record<GroupBy, string> = {
    none: "Tanpa grup",
    namespace: "Namespace",
    node: "Node",
    status: "Status",
};

const SOURCE_LABELS: Record<string, string> = {
    manual: "dipilih manual",
    KUBECONFIG: "env KUBECONFIG",
    home: "~/.kube/config",
    project: "<cwd>/.kube/config",
    none: "tidak ditemukan",
};

const OK_STATUSES = new Set(["Running", "Succeeded", "Completed"]);
const WARN_STATUSES = new Set([
    "Pending",
    "ContainerCreating",
    "PodInitializing",
    "NotReady",
    "Terminating",
]);

const TONE_CLASSES = {
    ok: "text-emerald-400",
    warn: "text-amber-400",
    bad: "text-red-400",
} as const;

function sourceLabel(source?: string): string {
    if (!source) {
        return "-";
    }
    return SOURCE_LABELS[source] ?? source;
}

function statusTone(status: string): "ok" | "warn" | "bad" {
    if (OK_STATUSES.has(status)) {
        return "ok";
    }
    if (WARN_STATUSES.has(status) || status.startsWith("Init:")) {
        return "warn";
    }
    return "bad";
}

function formatUpdatedAt(value?: string): string {
    if (!value) {
        return "-";
    }
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleTimeString();
}

function groupKeyOf(pod: kube.PodInfo, groupBy: GroupBy): string {
    switch (groupBy) {
        case "namespace":
            return pod.namespace;
        case "node":
            return pod.node;
        case "status":
            return pod.status;
        default:
            return "";
    }
}

// Status is plain coloured text rather than a badge: green while healthy, amber while the
// pod is still coming up, and red for failures such as CrashLoopBackOff, ImagePullBackOff,
// Error, Failed or Evicted. Nothing else in the UI uses colour.
function StatusLabel({status}: { status: string }) {
    return <span className={TONE_CLASSES[statusTone(status)]}>{status}</span>;
}

function App() {
    const [state, setState] = useState<main.AppState | null>(null);
    const [fatal, setFatal] = useState('');
    const [busy, setBusy] = useState(false);
    const [view, setView] = useState<View>("pod");
    const [namespace, setNamespace] = useState(ALL_NAMESPACES);
    const [query, setQuery] = useState('');
    const [groupBy, setGroupBy] = useState<GroupBy>("none");

    useEffect(() => {
        GetState()
            .then(setState)
            .catch((err) => setFatal(String(err)));

        // EventsOn returns an unsubscribe function. React StrictMode mounts effects
        // twice in development, so a leaked listener would duplicate every update.
        const off = EventsOn(STATE_UPDATE_EVENT, (next: main.AppState) => setState(next));

        return () => off();
    }, []);

    const run = useCallback(async (action: () => Promise<main.AppState>) => {
        setBusy(true);
        try {
            setState(await action());
            setFatal('');
        } catch (err) {
            setFatal(String(err));
        } finally {
            setBusy(false);
        }
    }, []);

    const config = state?.config;
    const pods = state?.pods ?? NO_PODS;
    const connected = state?.connected ?? false;
    const error = fatal || state?.error || '';
    const configMissing = config != null && config.path === '';

    const namespaces = useMemo(
        () => [...new Set(pods.map((pod) => pod.namespace))].sort(),
        [pods],
    );

    // A namespace can vanish when the kubeconfig changes or its pods are deleted, so fall
    // back to "all" instead of leaving the select on a value that no longer exists.
    const activeNamespace = namespaces.includes(namespace) ? namespace : ALL_NAMESPACES;

    // Filtering and grouping happen on the client because the snapshot is already in
    // memory, so they never trigger extra cluster requests.
    const filteredPods = useMemo(() => {
        const needle = query.trim().toLowerCase();

        return pods.filter((pod) => {
            if (activeNamespace !== ALL_NAMESPACES && pod.namespace !== activeNamespace) {
                return false;
            }
            return needle === '' || pod.name.toLowerCase().includes(needle);
        });
    }, [pods, activeNamespace, query]);

    const groups = useMemo(() => {
        if (groupBy === "none") {
            return [{key: "", pods: filteredPods}];
        }

        const byKey = new Map<string, kube.PodInfo[]>();
        for (const pod of filteredPods) {
            const key = groupKeyOf(pod, groupBy);
            const bucket = byKey.get(key);
            if (bucket) {
                bucket.push(pod);
            } else {
                byKey.set(key, [pod]);
            }
        }

        // Rows keep the namespace/name ordering the backend already applied.
        return [...byKey.entries()]
            .sort(([left], [right]) => left.localeCompare(right))
            .map(([key, grouped]) => ({key, pods: grouped}));
    }, [filteredPods, groupBy]);

    const filtering = activeNamespace !== ALL_NAMESPACES || query.trim() !== '';

    const resetFilters = useCallback(() => {
        setNamespace(ALL_NAMESPACES);
        setQuery('');
        setGroupBy("none");
    }, []);

    return (
        <SidebarProvider className="h-svh overflow-hidden">
            <Sidebar>
                <SidebarHeader>
                    <div className="flex items-center gap-2 px-2 py-1">
                        <Boxes className="size-5"/>
                        <div className="grid leading-tight">
                            <span className="font-semibold">antek-async</span>
                            <span className="text-muted-foreground">Kubernetes monitor</span>
                        </div>
                    </div>
                </SidebarHeader>
                <SidebarContent>
                    <SidebarGroup>
                        <SidebarGroupLabel>Monitoring</SidebarGroupLabel>
                        <SidebarGroupContent>
                            <SidebarMenu>
                                <SidebarMenuItem>
                                    <SidebarMenuButton
                                        isActive={view === "pod"}
                                        onClick={() => setView("pod")}
                                        tooltip="Pod"
                                    >
                                        <Boxes/>
                                        <span>Pod</span>
                                    </SidebarMenuButton>
                                </SidebarMenuItem>
                            </SidebarMenu>
                        </SidebarGroupContent>
                    </SidebarGroup>
                </SidebarContent>
                <SidebarFooter>
                    <SidebarMenu>
                        <SidebarMenuItem>
                            <SidebarMenuButton
                                isActive={view === "settings"}
                                onClick={() => setView("settings")}
                                tooltip="Settings"
                            >
                                <Settings/>
                                <span>Settings</span>
                            </SidebarMenuButton>
                        </SidebarMenuItem>
                    </SidebarMenu>
                </SidebarFooter>
            </Sidebar>

            <SidebarInset className="flex min-h-0 flex-col overflow-hidden">
                <header className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
                    <SidebarTrigger/>
                    <Separator orientation="vertical" className="h-5"/>
                    <div className="grid leading-tight">
                        <span className="font-semibold">{view === "pod" ? "Pod" : "Settings"}</span>
                        <span className="text-muted-foreground">
                            {view === "pod"
                                ? activeNamespace === ALL_NAMESPACES
                                    ? "Semua namespace"
                                    : activeNamespace
                                : "Kubeconfig dan koneksi"}
                        </span>
                    </div>
                    <div className="ml-auto flex items-center gap-2">
                        <Button onClick={() => run(PickKubeconfig)} disabled={busy} size="sm">
                            Pilih kubeconfig
                        </Button>
                        <Button
                            onClick={() => run(ResetKubeconfig)}
                            disabled={busy}
                            size="sm"
                            variant="outline"
                        >
                            Auto
                        </Button>
                        <Button
                            onClick={() => run(RefreshPods)}
                            disabled={busy}
                            size="sm"
                            variant="outline"
                        >
                            <RotateCw/>
                            {connected ? "Muat ulang" : "Coba lagi"}
                        </Button>
                    </div>
                </header>

                {error !== '' && (
                    <div className="mx-4 mt-4 flex shrink-0 items-start gap-3 rounded-lg border border-foreground/30 bg-muted px-3 py-2">
                        <div className="min-w-0 flex-1">
                            <p className="font-medium">Gagal menampilkan Pod</p>
                            <p className="mt-1 break-words text-muted-foreground">{error}</p>
                        </div>
                        <Button
                            onClick={() => run(RefreshPods)}
                            disabled={busy}
                            size="sm"
                            variant="outline"
                        >
                            Coba lagi
                        </Button>
                    </div>
                )}

                {view === "settings" ? (
                    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
                        <section className="rounded-lg border p-4">
                            <h2 className="font-medium">Koneksi</h2>
                            <div className="mt-3 grid gap-2">
                                <Field
                                    label="Status"
                                    value={connected ? "Live" : "Terputus"}
                                />
                                <Field label="Sumber" value={sourceLabel(config?.source)}/>
                                <Field label="File" value={config?.path || "-"} mono/>
                                <Field label="Context" value={config?.context || "-"}/>
                                <Field label="Cluster" value={config?.cluster || "-"}/>
                                <Field label="Server" value={config?.server || "-"} mono/>
                                <Field
                                    label="Update"
                                    value={formatUpdatedAt(state?.updatedAt)}
                                />
                            </div>
                        </section>

                        {configMissing && config != null && (
                            <section className="rounded-lg border p-4">
                                <h2 className="font-medium">Kubeconfig tidak ditemukan</h2>
                                <ul className="mt-3 space-y-1">
                                    {config.candidates.map((candidate) => (
                                        <li key={`${candidate.source}-${candidate.path}`}>
                                            <code className="font-mono">{candidate.path}</code>{" "}
                                            <span className="text-muted-foreground">
                                                ({sourceLabel(candidate.source)}
                                                {candidate.exists ? ", ada" : ", tidak ada"})
                                            </span>
                                        </li>
                                    ))}
                                </ul>
                            </section>
                        )}

                        <p className="text-muted-foreground">
                            Pengaturan lain belum tersedia.
                        </p>
                    </div>
                ) : (
                    <div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
                        <div className="flex flex-wrap items-center gap-2">
                            <Input
                                className="w-64"
                                onChange={(event) => setQuery(event.target.value)}
                                placeholder="Filter nama Pod"
                                value={query}
                            />

                            <Select onValueChange={setNamespace} value={activeNamespace}>
                                <SelectTrigger className="w-52">
                                    <SelectValue placeholder="Semua namespace"/>
                                </SelectTrigger>
                                <SelectContent>
                                    <SelectItem value={ALL_NAMESPACES}>Semua namespace</SelectItem>
                                    {namespaces.map((item) => (
                                        <SelectItem key={item} value={item}>
                                            {item}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>

                            <Select
                                onValueChange={(value) => setGroupBy(value as GroupBy)}
                                value={groupBy}
                            >
                                <SelectTrigger className="w-44">
                                    <SelectValue placeholder="Group By"/>
                                </SelectTrigger>
                                <SelectContent>
                                    {(Object.keys(GROUP_BY_LABELS) as GroupBy[]).map((option) => (
                                        <SelectItem key={option} value={option}>
                                            {GROUP_BY_LABELS[option]}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>

                            {filtering && (
                                <Button onClick={resetFilters} size="sm" variant="ghost">
                                    Reset filter
                                </Button>
                            )}
                        </div>

                        <div className="min-h-0 flex-1 overflow-hidden rounded-lg border">
                            <Table>
                                <TableHeader className="sticky top-0 z-10 bg-background">
                                    <TableRow>
                                        <TableHead>Namespace</TableHead>
                                        <TableHead>Name</TableHead>
                                        <TableHead className="text-right">Ready</TableHead>
                                        <TableHead>Status</TableHead>
                                        <TableHead className="text-right">Restarts</TableHead>
                                        <TableHead>Age</TableHead>
                                        <TableHead>Node</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {groups.map((group) => (
                                        <Fragment key={group.key || NO_GROUPING}>
                                            {groupBy !== "none" && (
                                                <TableRow className="bg-muted/40 hover:bg-muted/40">
                                                    <TableCell className="font-medium" colSpan={COLUMN_COUNT}>
                                                        {group.key}
                                                        <span className="ml-2 text-muted-foreground">
                                                            {group.pods.length}
                                                        </span>
                                                    </TableCell>
                                                </TableRow>
                                            )}
                                            {group.pods.map((pod) => (
                                                <TableRow key={`${pod.namespace}/${pod.name}`}>
                                                    <TableCell className="text-muted-foreground">
                                                        {pod.namespace}
                                                    </TableCell>
                                                    <TableCell className="font-medium">{pod.name}</TableCell>
                                                    <TableCell className="text-right tabular-nums">
                                                        {pod.ready}
                                                    </TableCell>
                                                    <TableCell>
                                                        <StatusLabel status={pod.status}/>
                                                    </TableCell>
                                                    <TableCell className="text-right tabular-nums">
                                                        {pod.restarts}
                                                    </TableCell>
                                                    <TableCell className="text-muted-foreground">
                                                        {pod.age}
                                                    </TableCell>
                                                    <TableCell className="text-muted-foreground">
                                                        {pod.node}
                                                    </TableCell>
                                                </TableRow>
                                            ))}
                                        </Fragment>
                                    ))}
                                </TableBody>
                            </Table>

                            {filteredPods.length === 0 && (
                                <p className="py-10 text-center text-muted-foreground">
                                    {state == null
                                        ? "Menghubungkan…"
                                        : pods.length === 0
                                            ? connected
                                                ? "Tidak ada Pod di cluster ini"
                                                : "Menunggu koneksi…"
                                            : "Tidak ada Pod yang cocok dengan filter"}
                                </p>
                            )}
                        </div>
                    </div>
                )}
            </SidebarInset>
        </SidebarProvider>
    )
}

function Field({label, value, mono}: { label: string; value: string; mono?: boolean }) {
    return (
        <span className="flex items-center gap-2">
            <span className="w-24 shrink-0 tracking-wide text-muted-foreground uppercase">
                {label}
            </span>
            <span className={mono ? "font-mono" : undefined}>{value}</span>
        </span>
    );
}

export default App
