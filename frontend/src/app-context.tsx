import { createContext, useContext } from "react";
import type { main } from "../wailsjs/go/models";

// The shell owns every piece of state the pages need, so pages read it from one context
// instead of threading props through the router.
export type AppContextValue = {
	state: main.AppState | null;
	busy: boolean;
	error: string;
	run: (action: () => Promise<main.AppState>) => Promise<void>;
	// The mounted page lends the shell its reconnect function, so the header's Reload button
	// never has to know which kind is on screen. null means there is nothing to reload.
	reload: (() => void) | null;
	setReload: (reload: (() => void) | null) => void;
};

export const AppContext = createContext<AppContextValue | null>(null);

export function useApp(): AppContextValue {
	const value = useContext(AppContext);
	if (value == null) {
		throw new Error("useApp must be used inside the app shell");
	}
	return value;
}
