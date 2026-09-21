// UsageCell stacks the two numbers metrics-server reports in a single column: CPU on top,
// memory under it, both right aligned so the digits line up down the table. A row without
// metrics reads "-" twice, which is what a cluster without metrics-server shows for every row.
export function UsageCell({
	usage,
}: {
	usage?: { cpu: string; memory: string };
}) {
	return (
		<span className="flex flex-col leading-tight">
			<span>CPU: {usage?.cpu ?? "-"}</span>
			<span className="text-muted-foreground">
				Memory {usage?.memory ?? "-"}
			</span>
		</span>
	);
}
