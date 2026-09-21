import { useCallback, useEffect, useRef } from "react";
import { SelectResource } from "../wailsjs/go/main/App";
import { type AppContextValue, useApp } from "@/app-context";

export type ResourceHandle = AppContextValue & { reload: () => void };

// useResource makes a page own the cluster kind it streams: the page starts the watch when it
// mounts and lends the shell a matching reload. That way `App.tsx` never names a resource, and
// only the page that is on screen talks to the cluster. Pages read the snapshot from the value
// this returns.
export function useResource(resource: string): ResourceHandle {
	const app = useApp();
	const { run, setReload } = app;
	const requested = useRef("");

	const reload = useCallback(() => {
		requested.current = resource;
		run(() => SelectResource(resource));
	}, [resource, run]);

	// StrictMode runs mount effects twice in dev, so the ref makes the repeat a no-op instead of
	// cancelling and restarting the same watch.
	useEffect(() => {
		if (requested.current === resource) {
			return;
		}
		requested.current = resource;
		run(() => SelectResource(resource));
	}, [resource, run]);

	// The header lives in the shell, so lend it this page's reload and take it back on unmount.
	useEffect(() => {
		setReload(reload);
		return () => setReload(null);
	}, [reload, setReload]);

	return { ...app, reload };
}
