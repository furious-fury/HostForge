import { AnchorHTMLAttributes, ReactNode } from "react";
import { Link, LinkProps } from "react-router-dom";
import { Button as ShadcnButton, buttonVariants } from "./ui/button";

export type ButtonVariant = "primary" | "secondary" | "danger" | "ghost";
export type ButtonSize = "sm" | "md";

type CommonProps = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  children: ReactNode;
};

type NativeButtonProps = React.ComponentPropsWithoutRef<typeof ShadcnButton> & CommonProps;

export function Button({ variant = "secondary", size = "md", className = "", children, ...rest }: NativeButtonProps) {
  return <ShadcnButton variant={variant} size={size} className={className} {...rest}>{children}</ShadcnButton>;
}

type RouterLinkProps = Omit<LinkProps, "children"> & CommonProps;

export function ButtonLink({ variant = "secondary", size = "md", className = "", children, ...rest }: RouterLinkProps) {
  return (
    <Link
      {...rest}
      className={buttonVariants({ variant, size, className })}
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
      className={buttonVariants({ variant, size, className })}
    >
      {children}
    </a>
  );
}
