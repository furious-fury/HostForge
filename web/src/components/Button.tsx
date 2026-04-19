import { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode } from "react";
import { Link, LinkProps } from "react-router-dom";

export type ButtonVariant = "primary" | "secondary" | "danger" | "ghost";
export type ButtonSize = "sm" | "md";

const baseClasses =
  "inline-flex items-center justify-center gap-2 border font-semibold uppercase tracking-wider transition-colors duration-100 select-none disabled:opacity-50 disabled:cursor-not-allowed";

const sizeClasses: Record<ButtonSize, string> = {
  sm: "px-3 py-1.5 text-[11px]",
  md: "px-4 py-2 text-xs",
};

const variantClasses: Record<ButtonVariant, string> = {
  primary:
    "border-primary bg-primary text-primary-ink hover:brightness-110",
  secondary:
    "border-border-strong bg-transparent text-text hover:border-muted hover:bg-border active:brightness-95",
  danger:
    "border-danger bg-transparent text-danger hover:bg-danger hover:text-primary-ink",
  ghost:
    "border-transparent bg-transparent text-muted hover:text-text hover:bg-surface-alt",
};

type CommonProps = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  children: ReactNode;
};

type NativeButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & CommonProps;

export function Button({ variant = "secondary", size = "md", className = "", children, ...rest }: NativeButtonProps) {
  return (
    <button
      {...rest}
      className={`${baseClasses} ${sizeClasses[size]} ${variantClasses[variant]} ${className}`}
    >
      {children}
    </button>
  );
}

type RouterLinkProps = Omit<LinkProps, "children"> & CommonProps;

export function ButtonLink({ variant = "secondary", size = "md", className = "", children, ...rest }: RouterLinkProps) {
  return (
    <Link
      {...rest}
      className={`${baseClasses} ${sizeClasses[size]} ${variantClasses[variant]} ${className}`}
    >
      {children}
    </Link>
  );
}

type AnchorProps = AnchorHTMLAttributes<HTMLAnchorElement> & CommonProps;

export function ButtonAnchor({ variant = "secondary", size = "md", className = "", children, ...rest }: AnchorProps) {
  return (
    <a
      {...rest}
      className={`${baseClasses} ${sizeClasses[size]} ${variantClasses[variant]} ${className}`}
    >
      {children}
    </a>
  );
}
