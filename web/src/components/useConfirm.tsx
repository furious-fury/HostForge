import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { ConfirmDialog, type ConfirmDialogProps } from "./ConfirmDialog";

type ConfirmOptions = Omit<ConfirmDialogProps, "open" | "onConfirm" | "onClose">;

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * Call `confirm({ title, description, ... })` to show a ConfirmDialog and
 * await the user's response. Resolves `true` on confirm, `false` on cancel.
 *
 * Must be rendered inside `<ConfirmProvider>`.
 */
export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext);
  if (!fn) {
    throw new Error("useConfirm() must be used inside <ConfirmProvider>");
  }
  return fn;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [dialog, setDialog] = useState<(ConfirmOptions & { open: boolean }) | null>(null);
  const resolveRef = useRef<((value: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
      setDialog({ ...opts, open: true });
    });
  }, []);

  const close = useCallback((result: boolean) => {
    resolveRef.current?.(result);
    resolveRef.current = null;
    setDialog((prev) => (prev ? { ...prev, open: false } : null));
  }, []);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {dialog && (
        <ConfirmDialog
          {...dialog}
          onConfirm={() => close(true)}
          onClose={() => close(false)}
        />
      )}
    </ConfirmContext.Provider>
  );
}
