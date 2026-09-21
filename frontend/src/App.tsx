import {
	type ComponentType,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";
import {
	HashRouter,
	Link,
	Navigate,
	Route,
	Routes,
	useLocation,
} from "react-router-dom";
import {
	Box,
	Boxes,
	Database,
	Layers,
	RotateCw,
	Server,
	Settings,
	SquarePen,
} from "lucide-react";
import {
	GetState,
	PickKubeconfig,
	ResetKubeconfig,
} from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";
import { main } from "../wailsjs/go/models";
import { AppContext, type AppContextValue } from "@/app-context";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
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
import { DeploymentsPage } from "@/pages/deployments";
import { EditorPage } from "@/pages/editor";
import { NodesPage } from "@/pages/nodes";
import { PodsPage } from "@/pages/pods";
import { SettingsPage } from "@/pages/settings";
import { StatefulSetsPage } from "@/pages/statefulsets";
import { DEFAULT_PATH, ROUTES, routeForPath, type View } from "@/routes";

const STATE_UPDATE_EVENT = "state:update";

// Nodes are cluster scoped and get their own group above the namespaced workloads. The editor
// writes rather than lists, so it sits in its own group below them.
const MENU_GROUPS: { label: string; views: View[] }[] = [
	{ label: "Cluster", views: ["node"] },
	{ label: "Workloads", views: ["pod", "deployment", "statefulset"] },
	{ label: "Manifest", views: ["editor"] },
];

// Icons are presentation only, so they stay out of the route metadata.
const VIEW_ICONS: Record<View, ComponentType<{ className?: string }>> = {
	node: Server,
	pod: Box,
	deployment: Layers,
	statefulset: Database,
	editor: SquarePen,
	settings: Settings,
};

// AppShell is the layout and the shared state; the pages themselves decide which cluster kind
// is streamed, so the shell only routes and renders chrome.
function AppShell() {
	const [state, setState] = useState<main.AppState | null>(null);
	const [fatal, setFatal] = useState("");
	const [busy, setBusy] = useState(false);

	const location = useLocation();
	const route = routeForPath(location.pathname);

	useEffect(() => {
		GetState()
			.then(setState)
			.catch((err) => setFatal(String(err)));

		// EventsOn returns an unsubscribe function. React StrictMode mounts effects
		// twice in development, so a leaked listener would duplicate every update.
		const off = EventsOn(STATE_UPDATE_EVENT, (next: main.AppState) =>
			setState(next),
		);

		return () => off();
	}, []);

	const run = useCallback(async (action: () => Promise<main.AppState>) => {
		setBusy(true);
		try {
			setState(await action());
			setFatal("");
		} catch (err) {
			setFatal(String(err));
		} finally {
			setBusy(false);
		}
	}, []);

	// The mounted page publishes its reconnect function through `useResource`, so the header can
	// offer Reload without the shell knowing which kind is on screen.
	const [reload, setReloadState] = useState<(() => void) | null>(null);
	const setReload = useCallback((next: (() => void) | null) => {
		setReloadState(() => next);
	}, []);

	const error = fatal || state?.error || "";

	const context: AppContextValue = useMemo(
		() => ({ busy, error, reload, run, setReload, state }),
		[busy, error, reload, run, setReload, state],
	);

	// Clicking the menu of the page that is already open reconnects it, which gives the sidebar
	// the same refresh the header button offers.
	const reloadIfOpen = useCallback(
		(view: View) => (route?.view === view ? (reload ?? undefined) : undefined),
		[route, reload],
	);

	return (
		<AppContext.Provider value={context}>
			<SidebarProvider className="h-svh overflow-hidden">
				<Sidebar>
					<SidebarHeader>
						<div className="flex items-center gap-2 px-2 py-1">
							<Boxes className="size-5" />
							<div className="grid leading-tight">
								<span className="font-semibold">antek-async</span>
								<span className="text-muted-foreground">
									Kubernetes monitor
								</span>
							</div>
						</div>
					</SidebarHeader>
					<SidebarContent>
						{MENU_GROUPS.map((group) => (
							<SidebarGroup key={group.label}>
								<SidebarGroupLabel>{group.label}</SidebarGroupLabel>
								<SidebarGroupContent>
									<SidebarMenu>
										{group.views.map((view) => {
											const item = ROUTES[view];
											const Icon = VIEW_ICONS[view];
											return (
												<SidebarMenuItem key={view}>
													<SidebarMenuButton
														asChild
														isActive={route?.view === view}
														tooltip={item.label}
													>
														<Link onClick={reloadIfOpen(view)} to={item.path}>
															<Icon />
															<span>{item.label}</span>
														</Link>
													</SidebarMenuButton>
												</SidebarMenuItem>
											);
										})}
									</SidebarMenu>
								</SidebarGroupContent>
							</SidebarGroup>
						))}
					</SidebarContent>
					<SidebarFooter>
						<SidebarMenu>
							<SidebarMenuItem>
								<SidebarMenuButton
									asChild
									isActive={route?.view === "settings"}
									tooltip="Settings"
								>
									<Link to={ROUTES.settings.path}>
										<Settings />
										<span>Settings</span>
									</Link>
								</SidebarMenuButton>
							</SidebarMenuItem>
						</SidebarMenu>
					</SidebarFooter>
				</Sidebar>

				<SidebarInset className="flex min-h-0 flex-col overflow-hidden">
					<header className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
						<SidebarTrigger />
						<Separator orientation="vertical" className="h-5" />
						<div className="grid leading-tight">
							<span className="font-semibold">
								{route?.label ?? ROUTES.pod.label}
							</span>
							<span className="text-muted-foreground">
								{route?.subtitle ?? ""}
							</span>
						</div>
						<div className="ml-auto flex items-center gap-2">
							<Button
								disabled={busy}
								onClick={() => run(PickKubeconfig)}
								size="sm"
							>
								Choose kubeconfig
							</Button>
							<Button
								disabled={busy}
								onClick={() => run(ResetKubeconfig)}
								size="sm"
								variant="outline"
							>
								Auto
							</Button>
							{reload != null && (
								<Button
									disabled={busy}
									onClick={reload}
									size="sm"
									variant="outline"
								>
									<RotateCw />
									Reload
								</Button>
							)}
						</div>
					</header>

					{error !== "" && (
						<div className="mx-4 mt-4 shrink-0 rounded-lg border border-foreground/30 bg-muted px-3 py-2">
							<p className="font-medium">Failed to load configuration</p>
							<p className="mt-1 break-words text-muted-foreground">{error}</p>
						</div>
					)}

					<Routes>
						<Route element={<PodsPage />} path={ROUTES.pod.path} />
						<Route
							element={<DeploymentsPage />}
							path={ROUTES.deployment.path}
						/>
						<Route
							element={<StatefulSetsPage />}
							path={ROUTES.statefulset.path}
						/>
						<Route element={<NodesPage />} path={ROUTES.node.path} />
						<Route element={<EditorPage />} path={ROUTES.editor.path} />
						<Route element={<SettingsPage />} path={ROUTES.settings.path} />
						<Route element={<Navigate replace to={DEFAULT_PATH} />} path="*" />
					</Routes>
				</SidebarInset>
			</SidebarProvider>
		</AppContext.Provider>
	);
}

export default function App() {
	return (
		<HashRouter>
			<AppShell />
		</HashRouter>
	);
}
