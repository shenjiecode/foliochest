import { useId } from "react";

export function ChoiceField<T extends string>({
  id,
  label,
  value,
  options,
  disabled = false,
  onChange,
}: {
  id: string;
  label: string;
  value: T;
  options: readonly { value: T; label: string }[];
  disabled?: boolean;
  onChange: (value: T) => void;
}) {
  const name = useId();
  return (
    <fieldset id={id} className="choice-field" disabled={disabled}>
      <legend>{label}</legend>
      <div className="choice-options">
        {options.map((option) => (
          <label className="choice-card" key={option.value}>
            <input
              type="radio"
              name={name}
              value={option.value}
              checked={value === option.value}
              onChange={() => onChange(option.value)}
            />
            <span>{option.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

export const formatChoices = [
  { value: "text", label: "纯文本" },
  { value: "markdown", label: "Markdown" },
  { value: "json", label: "JSON" },
] as const;
