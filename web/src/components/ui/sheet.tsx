import * as React from "react";
import { Dialog as SheetPrimitive } from "radix-ui";

function join(...classes: Array<string | undefined>) {
  return classes.filter(Boolean).join(" ");
}

const Sheet = SheetPrimitive.Root;
const SheetTrigger = SheetPrimitive.Trigger;
const SheetClose = SheetPrimitive.Close;

const SheetOverlay = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Overlay
    ref={ref}
    className={join("fixed inset-0 z-50 bg-black/65 backdrop-blur-[1px]", className)}
    {...props}
  />
));
SheetOverlay.displayName = SheetPrimitive.Overlay.displayName;

type SheetSide = "top" | "right" | "bottom" | "left";

const sideClasses: Record<SheetSide, string> = {
  top: "inset-x-0 top-0 border-b",
  right: "inset-y-0 right-0 h-full w-full border-l sm:max-w-2xl",
  bottom: "inset-x-0 bottom-0 border-t",
  left: "inset-y-0 left-0 h-full w-full border-r sm:max-w-2xl",
};

const SheetContent = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Content> & { side?: SheetSide }
>(({ side = "right", className, children, ...props }, ref) => (
  <SheetPrimitive.Portal>
    <SheetOverlay />
    <SheetPrimitive.Content
      ref={ref}
      className={join(
        "fixed z-50 flex flex-col overflow-hidden border-border bg-surface text-text shadow-2xl outline-none",
        sideClasses[side],
        className,
      )}
      {...props}
    >
      {children}
      <SheetPrimitive.Close
        className="absolute right-4 top-4 inline-flex size-8 items-center justify-center border border-border bg-surface-alt text-lg leading-none text-muted hover:border-border-strong hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
        aria-label="Close project settings"
      >
        ×
      </SheetPrimitive.Close>
    </SheetPrimitive.Content>
  </SheetPrimitive.Portal>
));
SheetContent.displayName = SheetPrimitive.Content.displayName;

function SheetHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={join("border-b border-border px-6 py-5 pr-14", className)} {...props} />;
}

function SheetTitle({ className, ...props }: React.ComponentPropsWithoutRef<typeof SheetPrimitive.Title>) {
  return <SheetPrimitive.Title className={join("text-lg font-semibold tracking-tight", className)} {...props} />;
}

function SheetDescription({ className, ...props }: React.ComponentPropsWithoutRef<typeof SheetPrimitive.Description>) {
  return <SheetPrimitive.Description className={join("mt-1 text-sm text-muted", className)} {...props} />;
}

export { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger };
