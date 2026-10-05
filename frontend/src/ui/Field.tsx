import { cloneElement, isValidElement, useId, type ReactElement, type ReactNode } from "react";

interface FieldProps {
  label: ReactNode;
  hint?: ReactNode;
  className?: string;
  children: ReactElement;
}

// Field labels one control and, when given, ties a hint to it through
// aria-describedby so the hint is announced after the label, not as part of it.
export function Field({ label, hint, className = "", children }: FieldProps) {
  const id = useId();
  const hintID = `${id}-hint`;
  const control = isValidElement(children)
    ? cloneElement(children as ReactElement<Record<string, unknown>>, {
        id,
        "aria-describedby": hint ? hintID : undefined,
      })
    : children;
  return (
    <div className={`field ${className}`}>
      <label htmlFor={id}>{label}</label>
      {control}
      {hint ? (
        <small id={hintID} className="field-hint">
          {hint}
        </small>
      ) : null}
    </div>
  );
}
