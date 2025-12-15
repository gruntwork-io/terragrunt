// A plain Button is often used as a link, so rather than wrapping plain button in an <a> tag, we can use this component.

import { cn } from "../../lib/utils";
import Button from "./Button";

// Extract the ButtonProps interface from Button component
type ButtonProps = React.ComponentProps<typeof Button>;

interface ButtonLinkProps extends ButtonProps {
  buttonClassName?: string;
  href?: string;
  rel?: string;
  target?: "_blank" | "_self" | "_parent" | "_top";
}

export default function ButtonLink({
  buttonClassName,
  href,
  rel,
  target,
  className,
  ...buttonProps
}: ButtonLinkProps) {

  return (
    <a
      href={href}
      target={target}
      rel={rel}
      className={cn(
        // Override default link styling
        "no-underline",
        "text-inherit",
        "hover:no-underline",
        "hover:text-inherit",
        // Ensure proper display
        "inline-block",
        className
      )}
    >
      <Button {...buttonProps} className={buttonClassName} />
    </a>
  );
}
