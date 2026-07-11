import { ReactNode, useMemo } from "react";
import { Toaster, toast as sonnerToast } from "sonner";

/** Brutalist toast chrome (Sonner `unstyled` + Tailwind). */
const toastClassNames = {
  toast:
    "group relative flex w-full items-start gap-3 border border-border bg-surface p-4 text-left text-sm text-text shadow-none",
  title: "pr-10 text-[0.9375rem] font-semibold leading-snug text-inherit",
  description: "mt-1 text-xs leading-relaxed text-muted",
  success: "!border-success !text-success",
  error: "!border-danger !text-danger",
  // Sonner’s stylesheet (if present) pins the close control top-left with translate(-35%,-35%); reset explicitly.
  closeButton:
    "!absolute !left-auto !right-2 !top-2 !bottom-auto !h-7 !w-7 !translate-x-0 !translate-y-0 !transform-none flex !items-center !justify-center border border-border bg-surface-alt p-0 text-muted hover:bg-border hover:text-text [&_svg]:size-3.5",
  icon: "mt-0.5 shrink-0 opacity-90",
} as const;

export type ToastContextValue = {
  success: (message: string) => void;
  error: (message: string) => void;
};

export function useToast(): ToastContextValue {
  return useMemo(
    () => ({
      success: (message: string) => {
        sonnerToast.success(message, { duration: 4500 });
      },
      error: (message: string) => {
        sonnerToast.error(message, { duration: 8000 });
      },
    }),
    [],
  );
}

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <>
      <Toaster
        position="top-right"
        closeButton
        gap={10}
        expand={false}
        visibleToasts={5}
        offset={{ top: "1rem", right: "1rem" }}
        toastOptions={{
          unstyled: true,
          classNames: toastClassNames,
        }}
        className="!z-[100]"
      />
      {children}
    </>
  );
}
