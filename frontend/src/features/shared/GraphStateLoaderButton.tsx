import { useRef, type ChangeEvent } from "react";

interface GraphStateLoaderButtonProps {
  className?: string;
  disabled?: boolean;
  label?: string;
  onLoadFile: (file: File) => void | Promise<void>;
}

export function GraphStateLoaderButton({
  className = "button secondary slim",
  disabled = false,
  label = "Load saved state",
  onLoadFile,
}: GraphStateLoaderButtonProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.currentTarget.value = "";
    if (!file) {
      return;
    }
    void onLoadFile(file);
  }

  return (
    <>
      <button
        type="button"
        className={className}
        disabled={disabled}
        onClick={() => inputRef.current?.click()}
      >
        {label}
      </button>
      <input ref={inputRef} type="file" accept=".json,application/json" hidden onChange={onFileChange} />
    </>
  );
}
