import type { DrawerResizeHandleProps } from "@/use-drawer-resize";

// DrawerResizeHandle is the strip along a drawer's left edge that drags it wider or narrower. It is
// a separator rather than a button, because it changes a size instead of being activated, and the
// drawer's own left border is the line it sits on.
export function DrawerResizeHandle(props: DrawerResizeHandleProps) {
	return (
		<div
			aria-label="Resize panel"
			aria-orientation="vertical"
			className="absolute inset-y-0 left-0 z-20 w-2 cursor-col-resize touch-none transition-colors hover:bg-border"
			role="separator"
			{...props}
		/>
	);
}
