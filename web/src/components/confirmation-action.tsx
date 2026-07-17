import { useState, type ReactElement } from "react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { cn } from "@/lib/utils"
import { Input } from "@/components/ui/input"

type ConfirmationActionProps = {
  trigger: ReactElement
  title: string
  description: string
  confirmLabel: string
  destructive?: boolean
  onConfirm?: () => void
  confirmationText?: string
}

export function ConfirmationAction({ trigger, title, description, confirmLabel, destructive = false, onConfirm, confirmationText }: ConfirmationActionProps) {
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState("")
  const changeOpen = (next: boolean) => { setOpen(next); if (!next) setTyped("") }
  const confirmed = !confirmationText || typed === confirmationText
  return <AlertDialog open={open} onOpenChange={changeOpen}><AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger><AlertDialogContent className="border-border bg-card"><AlertDialogHeader><AlertDialogTitle>{title}</AlertDialogTitle><AlertDialogDescription className="leading-6">{description}</AlertDialogDescription></AlertDialogHeader>{confirmationText && <label className="block text-xs font-medium">Type <span className="font-mono">{confirmationText}</span> to confirm<Input className="mt-2 font-mono" value={typed} onChange={(event) => setTyped(event.target.value)} autoComplete="off" /></label>}<AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction disabled={!confirmed} onClick={onConfirm} className={cn(destructive && "bg-destructive text-destructive-foreground hover:bg-destructive/90")}>{confirmLabel}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
}
