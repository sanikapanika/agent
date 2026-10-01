import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import type { ConfigValue, SchemaField } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Field, Input, Select, Switch, Textarea } from "@/components/ui/input";

/**
 * Renders a type's settings from its description (internal/schema.Field), so
 * monitor types and notification channels added in Go need no UI changes.
 */
export function SchemaFields({
  fields,
  values,
  onChange,
  idPrefix,
}: {
  fields: SchemaField[];
  values: Record<string, ConfigValue>;
  onChange: (key: string, value: ConfigValue) => void;
  idPrefix: string;
}) {
  if (fields.length === 0) return null;
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {fields.map((f) => {
        const id = `${idPrefix}-${f.key}`;
        const value = values[f.key] ?? f.default;
        const wide = cn(f.wide && "sm:col-span-2");
        if (f.input === "switch") {
          return (
            <div key={f.key} className={cn("flex flex-col gap-1", wide)}>
              <Switch id={id} checked={value === true} onChange={(v) => onChange(f.key, v)} label={f.label} />
              {f.hint && <p className="text-xs text-muted-foreground">{f.hint}</p>}
            </div>
          );
        }
        return (
          <Field key={f.key} label={f.label} htmlFor={id} hint={f.hint} className={wide}>
            <FieldInput id={id} field={f} value={value} onChange={(v) => onChange(f.key, v)} />
          </Field>
        );
      })}
    </div>
  );
}

function FieldInput({
  id,
  field: f,
  value,
  onChange,
}: {
  id: string;
  field: SchemaField;
  value: ConfigValue;
  onChange: (value: ConfigValue) => void;
}) {
  const text = value === undefined || typeof value === "object" ? "" : String(value);
  switch (f.input) {
    case "select":
      return (
        <Select id={id} required={f.required} value={text} onChange={(e) => onChange(e.target.value)}>
          {f.options?.map((o) => (
            <option key={o}>{o}</option>
          ))}
        </Select>
      );
    case "number":
      return (
        <Input
          id={id}
          type="number"
          required={f.required}
          placeholder={f.placeholder}
          value={text}
          onChange={(e) => onChange(e.target.value === "" ? undefined : Number(e.target.value))}
        />
      );
    case "textarea":
      return (
        <Textarea
          id={id}
          required={f.required}
          placeholder={f.placeholder}
          value={text}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "password":
      return <SecretInput id={id} required={f.required} placeholder={f.placeholder} value={text} onChange={onChange} />;
    default:
      return (
        <Input
          id={id}
          required={f.required}
          placeholder={f.placeholder}
          spellCheck={false}
          value={text}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

/** A secret (webhook URL, bot token): hidden by default, with a toggle to check what was pasted. */
function SecretInput({
  id,
  required,
  placeholder,
  value,
  onChange,
}: {
  id: string;
  required?: boolean;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const [shown, setShown] = useState(false);
  return (
    <div className="relative">
      <Input
        id={id}
        type={shown ? "text" : "password"}
        autoComplete="off"
        spellCheck={false}
        required={required}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="pr-10"
      />
      <button
        type="button"
        aria-label={shown ? "Hide" : "Show"}
        onClick={() => setShown((s) => !s)}
        className="absolute top-1/2 right-1 grid size-7 -translate-y-1/2 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        {shown ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}
