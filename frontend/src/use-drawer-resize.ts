import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";

// The width a pod session drawer opens with. It matches the 48rem the drawer was pinned at before
// it could be dragged, so a drawer nobody resizes looks exactly as it did.
export const DEFAULT_DRAWER_WIDTH = 768;

// Narrow enough that the drawer never becomes a sliver, and wide enough for a log line to have room.
const MIN_DRAWER_WIDTH = 320;

// What stays visible of the page behind the drawer, so it is always obvious how to get back to it.
const DRAWER_EDGE_MARGIN = 96;

export type DrawerResizeHandleProps = {
	onPointerDown: (event: ReactPointerEvent<HTMLDivElement>) => void;
	onPointerMove: (event: ReactPointerEvent<HTMLDivElement>) => void;
	onPointerUp: (event: ReactPointerEvent<HTMLDivElement>) => void;
	onPointerCancel: (event: ReactPointerEvent<HTMLDivElement>) => void;
};

// useDrawerResize makes a right-hand drawer draggable at its left edge. The width cannot live in the
// CSS because it changes while the pointer moves, so it comes back as an inline style; that also
// beats both the Drawer's own `w-3/4` / `sm:max-w-sm` and the unlayered rule in style.css, which a
// class passed at the call site could not.
export function useDrawerResize(defaultWidth = DEFAULT_DRAWER_WIDTH): {
	style: CSSProperties;
	handleProps: DrawerResizeHandleProps;
} {
	const [width, setWidth] = useState(() => clampWidth(defaultWidth));
	const drag = useRef<{
		pointerId: number;
		startX: number;
		startWidth: number;
	} | null>(null);

	// A window that shrinks must not leave the drawer wider than the screen.
	useEffect(() => {
		const onWindowResize = () => setWidth((current) => clampWidth(current));
		window.addEventListener("resize", onWindowResize);
		return () => window.removeEventListener("resize", onWindowResize);
	}, []);

	const onPointerDown = useCallback(
		(event: ReactPointerEvent<HTMLDivElement>) => {
			// The handle sits over the overlay, so the drawer must not also read this as its own drag
			// and the pointer must not start a text selection instead.
			event.preventDefault();
			event.stopPropagation();

			drag.current = {
				pointerId: event.pointerId,
				startX: event.clientX,
				startWidth: width,
			};
			// Capturing means the moves keep arriving even when the pointer outruns the handle.
			event.currentTarget.setPointerCapture(event.pointerId);
		},
		[width],
	);

	const onPointerMove = useCallback(
		(event: ReactPointerEvent<HTMLDivElement>) => {
			const current = drag.current;
			if (current === null || current.pointerId !== event.pointerId) {
				return;
			}

			// The drawer is anchored to the right, so moving the pointer left makes it wider.
			const delta = current.startX - event.clientX;
			setWidth(clampWidth(current.startWidth + delta));
		},
		[],
	);

	const endDrag = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
		if (drag.current?.pointerId !== event.pointerId) {
			return;
		}
		drag.current = null;

		if (event.currentTarget.hasPointerCapture(event.pointerId)) {
			event.currentTarget.releasePointerCapture(event.pointerId);
		}
	}, []);

	const style = useMemo<CSSProperties>(
		() => ({ maxWidth: width, width }),
		[width],
	);

	return {
		style,
		handleProps: {
			onPointerDown,
			onPointerMove,
			onPointerUp: endDrag,
			onPointerCancel: endDrag,
		},
	};
}

function clampWidth(width: number): number {
	// At least the minimum is always kept, even on a window too narrow to give up the margin.
	const maxWidth = Math.max(MIN_DRAWER_WIDTH, window.innerWidth - DRAWER_EDGE_MARGIN);

	return Math.min(Math.max(width, MIN_DRAWER_WIDTH), maxWidth);
}
