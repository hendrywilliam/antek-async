import { Fragment, type ReactNode, useCallback, useMemo, useState } from "react";
import { LoaderCircle } from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";

// Radix Select treats an empty string as "no value", so the unfiltered choices need
// their own sentinel values.
export const ALL_NAMESPACES = "__all__";
export const NO_GROUPING = "__none__";

export type Column<T> = {
	header: string;
	align?: "right";
	className?: string;
	render: (item: T) => ReactNode;
};

export type GroupOption<T> = {
	value: string;
	label: string;
	key: (item: T) => string;
};

export type Accessors<T> = {
	name: (item: T) => string;
	key: (item: T) => string;
	// Cluster-scoped kinds such as nodes have no namespace, which also hides the namespace
	// filter and namespace grouping for them.
	namespace?: (item: T) => string;
};

export function namespaceGroups<T>(
	namespaceOf: (item: T) => string,
): GroupOption<T>[] {
	return [
		{ value: NO_GROUPING, label: "No grouping", key: () => "" },
		{ value: "namespace", label: "Namespace", key: namespaceOf },
	];
}

// ResourceTable renders one resource kind with its own filter, namespace selector and
// Group By menu. Filtering and grouping stay on the client because the snapshot is already
// in memory, so they never trigger extra cluster requests.
export function ResourceTable<T>({
	rows,
	columns,
	groups: groupOptions,
	accessors,
	noun,
	loaded,
	loading,
	error,
	onRetry,
	rowActions,
	selectable,
	toolbar,
	notice = "",
}: {
	rows: T[];
	columns: Column<T>[];
	groups: GroupOption<T>[];
	accessors: Accessors<T>;
	noun: string;
	loaded: boolean;
	loading: boolean;
	error: string;
	onRetry: () => void;
	rowActions?: (item: T) => ReactNode;
	// Selection is opt-in, and a table that offers a bulk action passes `selectable` together
	// with a `toolbar`, which receives the ticked rows and is rendered above the table.
	selectable?: boolean;
	toolbar?: (selected: T[]) => ReactNode;
	// A page that loads something besides the rows, such as CPU and memory, explains an empty
	// extra column here rather than in a cell.
	notice?: string;
}) {
	const [namespace, setNamespace] = useState(ALL_NAMESPACES);
	const [query, setQuery] = useState("");
	const [groupBy, setGroupBy] = useState(NO_GROUPING);

	const namespaces = useMemo(
		() =>
			accessors.namespace
				? [...new Set(rows.map(accessors.namespace))].sort()
				: [],
		[rows, accessors],
	);

	// A namespace can vanish when the kubeconfig changes or its workloads are deleted, so
	// fall back to "all" instead of leaving the select on a value that no longer exists.
	const activeNamespace = namespaces.includes(namespace)
		? namespace
		: ALL_NAMESPACES;

	const visible = useMemo(() => {
		const needle = query.trim().toLowerCase();
		const namespaceOf = accessors.namespace;

		return rows.filter((row) => {
			if (
				namespaceOf &&
				activeNamespace !== ALL_NAMESPACES &&
				namespaceOf(row) !== activeNamespace
			) {
				return false;
			}
			return (
				needle === "" || accessors.name(row).toLowerCase().includes(needle)
			);
		});
	}, [rows, activeNamespace, query, accessors]);

	const groups = useMemo(() => {
		const option = groupOptions.find(
			(candidate) => candidate.value === groupBy,
		);
		if (!option) {
			return [{ key: "", rows: visible }];
		}

		const byKey = new Map<string, T[]>();

		for (const row of visible) {
			const key = option.key(row);
			const bucket = byKey.get(key);
			if (bucket) {
				bucket.push(row);
			} else {
				byKey.set(key, [row]);
			}
		}

		// Rows keep the namespace/name ordering the backend already applied.
		return [...byKey.entries()]
			.sort(([left], [right]) => left.localeCompare(right))
			.map(([key, grouped]) => ({ key, rows: grouped }));
	}, [visible, groupBy, groupOptions]);

	// Ticked rows are stored as keys and re-derived from the rows on every render, so a row
	// that a delete or a changed kubeconfig removed cannot stay selected.
	const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
	const selected = useMemo(
		() => rows.filter((row) => selectedKeys.has(accessors.key(row))),
		[rows, selectedKeys, accessors],
	);

	const visibleKeys = useMemo(
		() => visible.map(accessors.key),
		[visible, accessors],
	);
	const allVisibleSelected =
		visibleKeys.length > 0 && visibleKeys.every((key) => selectedKeys.has(key));
	const someVisibleSelected = visibleKeys.some((key) => selectedKeys.has(key));

	const setSelected = useCallback((keys: string[], next: boolean) => {
		setSelectedKeys((current) => {
			const updated = new Set(current);
			for (const key of keys) {
				if (next) {
					updated.add(key);
				} else {
					updated.delete(key);
				}
			}
			return updated;
		});
	}, []);

	const filtering =
		activeNamespace !== ALL_NAMESPACES ||
		query.trim() !== "" ||
		groupBy !== NO_GROUPING;

	const resetFilters = useCallback(() => {
		setNamespace(ALL_NAMESPACES);
		setQuery("");
		setGroupBy(NO_GROUPING);
	}, []);

	// Until the first fetch finishes there is nothing to filter, so show the spinner or the
	// failure instead of an empty table.
	if (!loaded) {
		return (
			<div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
				<div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border">
					{error !== "" ? (
						<div className="flex max-w-xl flex-col items-center gap-3 px-6 text-center">
							<p className="text-red-400">{error}</p>
							<Button onClick={onRetry} size="sm" variant="outline">
								Retry
							</Button>
						</div>
					) : loading ? (
						<p className="flex items-center gap-2 text-muted-foreground">
							<LoaderCircle className="size-4 animate-spin" />
							Loading {noun}…
						</p>
					) : (
						<p className="text-muted-foreground">Waiting…</p>
					)}
				</div>
			</div>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
			{toolbar && (
				<div className="flex flex-wrap items-center gap-2">
					{toolbar(selected)}
				</div>
			)}

			<div className="flex flex-wrap items-center gap-2">
				<Input
					className="w-64"
					onChange={(event) => setQuery(event.target.value)}
					placeholder={`Filter ${noun} name`}
					value={query}
				/>

				{accessors.namespace && (
					<Select onValueChange={setNamespace} value={activeNamespace}>
						<SelectTrigger className="w-52">
							<SelectValue placeholder="All namespaces" />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={ALL_NAMESPACES}>All namespaces</SelectItem>
							{namespaces.map((item) => (
								<SelectItem key={item} value={item}>
									{item}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				)}

				<Select onValueChange={setGroupBy} value={groupBy}>
					<SelectTrigger className="w-44">
						<SelectValue placeholder="Group By" />
					</SelectTrigger>
					<SelectContent>
						{groupOptions.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>

				{filtering && (
					<Button onClick={resetFilters} size="sm" variant="ghost">
						Reset filters
					</Button>
				)}

				{notice !== "" && (
					<span className="text-muted-foreground">{notice}</span>
				)}
			</div>

			<div className="min-h-0 flex-1 overflow-hidden rounded-lg border">
				<Table>
					<TableHeader className="sticky top-0 z-10 bg-background">
						<TableRow>
							{selectable && (
								<TableHead className="w-10">
									<Checkbox
										aria-label={`Select all ${noun}`}
										checked={
											allVisibleSelected
												? true
												: someVisibleSelected
													? "indeterminate"
													: false
										}
										onCheckedChange={(checked) =>
											setSelected(visibleKeys, checked === true)
										}
									/>
								</TableHead>
							)}
							{columns.map((column) => (
								<TableHead
									key={column.header}
									className={
										column.align === "right" ? "text-right" : undefined
									}
								>
									{column.header}
								</TableHead>
							))}
							{rowActions && <TableHead className="w-10" />}
						</TableRow>
					</TableHeader>
					<TableBody>
						{groups.map((group) => (
							<Fragment key={group.key || NO_GROUPING}>
								{groupBy !== NO_GROUPING && (
									<TableRow className="bg-muted/40 hover:bg-muted/40">
										<TableCell
											className="font-medium"
											colSpan={
												columns.length +
												(selectable ? 1 : 0) +
												(rowActions ? 1 : 0)
											}
										>
											{group.key}
											<span className="ml-2 text-muted-foreground">
												{group.rows.length}
											</span>
										</TableCell>
									</TableRow>
								)}
								{group.rows.map((row) => {
									const key = accessors.key(row);
									return (
										<TableRow key={key}>
											{selectable && (
												<TableCell className="w-10">
													<Checkbox
														aria-label={`Select ${accessors.name(row)}`}
														checked={selectedKeys.has(key)}
														onCheckedChange={(checked) =>
															setSelected([key], checked === true)
														}
													/>
												</TableCell>
											)}
											{columns.map((column) => (
												<TableCell
													key={column.header}
													className={cn(
														column.align === "right" &&
															"text-right tabular-nums",
														column.className,
													)}
												>
													{column.render(row)}
												</TableCell>
											))}
											{rowActions && <TableCell>{rowActions(row)}</TableCell>}
										</TableRow>
									);
								})}
							</Fragment>
						))}
					</TableBody>
				</Table>

				{visible.length === 0 && (
					<p className="py-10 text-center text-muted-foreground">
						{rows.length === 0
							? `No ${noun} in this cluster`
							: `No ${noun} matches the filter`}
					</p>
				)}
			</div>
		</div>
	);
}
