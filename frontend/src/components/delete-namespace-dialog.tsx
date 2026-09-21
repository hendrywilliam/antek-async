import { type FormEvent, useState } from "react";
import { LoaderCircle } from "lucide-react";
import { DeleteNamespace } from "../../wailsjs/go/main/App";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// ConfirmWord is what the user has to type before a namespace is deleted. Deleting is the only
// destructive action in the app and it cannot be undone, so it takes two matching inputs: this
// word and the name of the namespace itself.
const CONFIRM_WORD = "delete";

// DeleteNamespaceDialog deletes one namespace after the user types the confirmation word and the
// namespace name again. It is mounted per namespace (`key`), so the two inputs always start
// empty for the namespace on screen, and it reports the result itself: a rejected delete stays
// in line with the dialog, the same way the manifest page reports its own outcome.
export function DeleteNamespaceDialog({
	name,
	remaining,
	onCancel,
	onDeleted,
}: {
	name: string;
	remaining: number;
	onCancel: () => void;
	onDeleted: () => void;
}) {
	const [word, setWord] = useState("");
	const [typedName, setTypedName] = useState("");
	const [deleting, setDeleting] = useState(false);
	const [error, setError] = useState("");

	const confirm = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();

		if (word.trim() !== CONFIRM_WORD) {
			setError(`Type ${CONFIRM_WORD} to confirm.`);
			return;
		}
		// The name is what proves the user means this namespace and not the one next to it.
		if (typedName.trim() !== name) {
			setError("The namespace name does not match.");
			return;
		}

		setDeleting(true);
		setError("");

		try {
			await DeleteNamespace(name);
			onDeleted();
		} catch (err) {
			setError(String(err));
		} finally {
			setDeleting(false);
		}
	};

	return (
		<Dialog
			onOpenChange={(open) => {
				// Mid-delete the dialog stays put, so the outcome cannot land nowhere.
				if (!open && !deleting) {
					onCancel();
				}
			}}
			open
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>Delete namespace</DialogTitle>
					<DialogDescription>
						This removes <span className="font-mono">{name}</span> and
						everything in it. It cannot be undone.
						{remaining > 0 &&
							` ${remaining} more selected namespaces follow after this one.`}
					</DialogDescription>
				</DialogHeader>

				<form className="grid gap-4" onSubmit={confirm}>
					<div className="grid gap-2">
						<Label htmlFor="confirm-word">Type "delete" to confirm</Label>
						<Input
							disabled={deleting}
							id="confirm-word"
							onChange={(event) => setWord(event.target.value)}
							placeholder={CONFIRM_WORD}
							value={word}
						/>
					</div>

					<div className="grid gap-2">
						<Label htmlFor="confirm-name">Type "{name}" to confirm</Label>
						<Input
							disabled={deleting}
							id="confirm-name"
							onChange={(event) => setTypedName(event.target.value)}
							placeholder={name}
							value={typedName}
						/>
					</div>

					{error !== "" && <p className="break-words text-red-400">{error}</p>}

					<DialogFooter>
						<Button
							disabled={deleting}
							onClick={onCancel}
							type="button"
							variant="outline"
						>
							Cancel
						</Button>
						{/* No red button: the palette stays monochrome, so the weight of the text
						    carries the warning instead of a colour. */}
						<Button disabled={deleting} type="submit">
							{deleting && <LoaderCircle className="animate-spin" />}
							Confirm
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
