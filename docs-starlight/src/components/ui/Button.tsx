// Generated with 'npx shadcn@latest add button'
// Customize this as needed!

import { cn } from "../../lib/utils";
import { ExternalLink } from "lucide-react";

interface ButtonProps {
  // TODO: Style secondary, ghost, outline, and link bvariants
  variant?: "primary" | "secondary" | "ghost" | "outline" | "destructive" | "link";
  size?: "default" | "sm" | "lg" | "icon";
  className?: string;
  children: React.ReactNode;
  onClick?: () => void;
  type?: "button" | "submit" | "reset";
  isExternalLink?: boolean;
}

export default function Button({
  variant = "primary",
  size = "default",
  className,
  children,
  onClick,
  type = "button",
  isExternalLink = false,
  ...props
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        // Base styles
        "inline-block cursor-pointer h-[39px] pl-[24px] pr-[24px]",
        // Border
        "border border-solid",
        // Focus
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
        // Text
        "font-sans rounded-lg text-sm text-center leading-normal",
        // Box shadow
        "shadow-[0px_0px_15px_0px_rgba(137,107,255,0.30),0px_0px_1px_4px_rgba(52,49,27,0.22),0px_0px_1px_4px_rgba(52,49,47,0.22)]",
        // Outline properties
        "outline-none outline-offset-0",
        // Text decoration properties
        "no-underline decoration-solid decoration-auto",
        // Transitions with gradient variables and correct timing function
        "transition-[color,background-color,border-color,outline-color,text-decoration-color,fill,stroke,--tw-gradient-from,--tw-gradient-via,--tw-gradient-to] duration-150 ease-[cubic-bezier(0.4,0,0.2,1)]",
        {
          "bg-[var(--color-accent-1)] text-[var(--sl-color-white)] hover:bg-[var(--sl-color-accent)] border-[var(--color-button-primary-border)]": variant === "primary",
          "bg-red-500 text-white hover:bg-red-600 border-red-500": variant === "destructive",
          "border border-[var(--sl-color-docs-stroke)] bg-[var(--sl-color-bg)] hover:bg-[var(--sl-color-gray-6)]": variant === "outline",
          "bg-[var(--sl-color-gray-6)] text-[var(--sl-color-gray-1)] hover:bg-[var(--sl-color-gray-5)] border-[var(--sl-color-gray-6)]": variant === "secondary",
          "hover:bg-[var(--sl-color-gray-6)] border-transparent": variant === "ghost",
          "text-[var(--sl-color-accent)] underline-offset-4 hover:underline border-transparent": variant === "link",
        },
        {
          "w-fit": size === "default",
          "h-9 pt-2 pb-2 pl-3 pr-3 w-auto": size === "sm",
          "h-11 pt-2 pb-2 pl-8 pr-8 w-auto": size === "lg",
          "h-10 w-10 p-0": size === "icon",
        },
        className
      )}
      onClick={onClick}
      {...props}
    >
      {children}
      {isExternalLink && (
        <ExternalLink 
          className="inline-block ml-1 transform translate-y-0.25 -translate-x-0.75 w-3.5 h-3.5" 
          aria-label="External link icon"
        />
      )}
    </button>
  );
}
