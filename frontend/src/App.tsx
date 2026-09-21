import {useCallback, useEffect, useState} from 'react';
import './App.css';
import {GetState, PickKubeconfig, RefreshPods, ResetKubeconfig} from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime";
import {main} from "../wailsjs/go/models";

const STATE_UPDATE_EVENT = "state:update";

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

function sourceLabel(source?: string): string {
    if (!source) {
        return "-";
    }
    return SOURCE_LABELS[source] ?? source;
}

function statusTone(status: string): string {
    if (OK_STATUSES.has(status)) {
        return "status ok";
    }
    if (WARN_STATUSES.has(status) || status.startsWith("Init:")) {
        return "status warn";
    }
    return "status bad";
}

function formatUpdatedAt(value?: string): string {
    if (!value) {
        return "-";
    }
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleTimeString();
}

function App() {
    const [state, setState] = useState<main.AppState | null>(null);
    const [fatal, setFatal] = useState('');
    const [busy, setBusy] = useState(false);

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
    const pods = state?.pods ?? [];
    const connected = state?.connected ?? false;
    const error = fatal || state?.error || '';

    return (
        <div id="App">
            <header className="topbar">
                <div className="titles">
                    <h1>antek-async</h1>
                    <p className="subtitle">Pod di semua namespace</p>
                </div>
                <div className="actions">
                    <button className="btn" onClick={() => run(PickKubeconfig)} disabled={busy}>
                        Pilih kubeconfig…
                    </button>
                    <button className="btn ghost" onClick={() => run(ResetKubeconfig)} disabled={busy}>
                        Auto
                    </button>
                    <button className="btn ghost" onClick={() => run(RefreshPods)} disabled={busy}>
                        {connected ? "Muat ulang" : "Coba lagi"}
                    </button>
                </div>
            </header>

            <section className="configbar">
                <span className={`badge ${connected ? "live" : "down"}`}>
                    {connected ? "Live" : "Terputus"}
                </span>
                <span className="item">
                    <span className="label">Sumber</span>
                    {sourceLabel(config?.source)}
                </span>
                <span className="item">
                    <span className="label">File</span>
                    <code>{config?.path || "-"}</code>
                </span>
                <span className="item">
                    <span className="label">Context</span>
                    {config?.context || "-"}
                </span>
                <span className="item">
                    <span className="label">Cluster</span>
                    {config?.cluster || "-"}
                </span>
                <span className="item">
                    <span className="label">Server</span>
                    <code>{config?.server || "-"}</code>
                </span>
                <span className="item">
                    <span className="label">Update</span>
                    {formatUpdatedAt(state?.updatedAt)}
                </span>
            </section>

            {error !== '' && (
                <div className="banner">
                    <strong>Gagal menampilkan Pod</strong>
                    <p>{error}</p>
                    <button className="btn" onClick={() => run(RefreshPods)} disabled={busy}>
                        Coba lagi
                    </button>
                </div>
            )}

            {config != null && config.path === '' && (
                <div className="candidates">
                    <div>Kubeconfig tidak ditemukan. Lokasi yang dicek:</div>
                    <ul>
                        {config.candidates.map((candidate) => (
                            <li key={`${candidate.source}-${candidate.path}`}>
                                <code>{candidate.path}</code>{" "}
                                <span className="muted">
                                    ({sourceLabel(candidate.source)}
                                    {candidate.exists ? ", ada" : ", tidak ada"})
                                </span>
                            </li>
                        ))}
                    </ul>
                </div>
            )}

            {state == null ? (
                <div className="placeholder">Menghubungkan…</div>
            ) : pods.length === 0 ? (
                <div className="placeholder">
                    {connected ? "Tidak ada Pod di cluster ini" : "Menunggu koneksi…"}
                </div>
            ) : (
                <div className="tablewrap">
                    <table>
                        <thead>
                        <tr>
                            <th>Namespace</th>
                            <th>Name</th>
                            <th className="num">Ready</th>
                            <th>Status</th>
                            <th className="num">Restarts</th>
                            <th>Age</th>
                            <th>Node</th>
                        </tr>
                        </thead>
                        <tbody>
                        {pods.map((pod) => (
                            <tr key={`${pod.namespace}/${pod.name}`}>
                                <td className="ns">{pod.namespace}</td>
                                <td className="name">{pod.name}</td>
                                <td className="num">{pod.ready}</td>
                                <td>
                                    <span className={statusTone(pod.status)}>{pod.status}</span>
                                </td>
                                <td className="num">{pod.restarts}</td>
                                <td>{pod.age}</td>
                                <td>{pod.node}</td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                    <div className="count">{pods.length} Pod</div>
                </div>
            )}
        </div>
    )
}

export default App
