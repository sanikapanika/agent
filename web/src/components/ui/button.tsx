import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0 cursor-pointer",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive: "bg-down text-white hover:bg-down/90",
        outline: "border border-input bg-card hover:bg-muted",
        ghost: "hover:bg-muted",
      },
      size: {
        default: "h-9 px-4",
        sm: "h-8 px-3 text-xs",
        icon: "size-9",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export interface ButtonProps extends React.ComponentProps<"button">, VariantProps<typeof buttonVariants> {}

export function Button({ className, variant, size, type = "button", ...props }: ButtonProps) {
  return <button type={type} className={cn(buttonVariants({ variant, size }), className)} {...props} />;
}
