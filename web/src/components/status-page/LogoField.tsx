import { useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ImagePlus } from "lucide-react";
import { api, type StatusPageLogos } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/ui/toast";

const MAX_LOGO_BYTES = 512 * 1024;

/** Uploads (or removes) a logo straight away. */
export function LogoField({
  variant,
  label,
  hint,
  url,
  onChange,
  disabled,
}: {
  variant: "light" | "dark";
  label: string;
  hint: string;
  url?: string;
  onChange: (logos: StatusPageLogos) => void;
  disabled: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<string>();
  const upload = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > MAX_LOGO_BYTES) throw new Error("Keep the logo under 512 KB.");
      const data = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result).split(",")[1] ?? "");
        reader.onerror = () => reject(new Error("The file couldn't be read."));
        reader.readAsDataURL(file);
      });
      return api.uploadStatusPageLogo(variant, data);
    },
    onSuccess: (c) => {
      setError(undefined);
      onChange(c.logos);
      toast.success(`${label} updated`);
    },
    onError: (e) => setError(e.message),
  });
  const remove = useMutation({
    mutationFn: () => api.deleteStatusPageLogo(variant),
    onSuccess: (c) => {
      onChange(c.logos);
      toast.success(`${label} removed`);
    },
    onError: (e) => setError(e.message),
  });
  const busy = upload.isPending || remove.isPending;

  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm leading-none font-medium">{label}</span>
      <div
        className={cn(
          "flex h-20 items-center justify-center overflow-hidden rounded-lg border p-3",
          // Show each logo on the background it's meant for.
          variant === "dark" ? "bg-black" : "bg-white",
        )}
      >
        {url ? (
          <img src={url} alt="" className="max-h-full max-w-full object-contain" />
        ) : (
          <span className={cn("text-xs", variant === "dark" ? "text-neutral-400" : "text-neutral-500")}>No logo</span>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file) upload.mutate(file);
        }}
      />
      <div className="flex gap-2">
        <Button variant="outline" size="sm" disabled={disabled || busy} onClick={() => input.current?.click()}>
          <ImagePlus /> {upload.isPending ? "Uploading…" : url ? "Replace" : "Upload"}
        </Button>
        {url && (
          <Button variant="ghost" size="sm" disabled={disabled || busy} onClick={() => remove.mutate()}>
            Remove
          </Button>
        )}
      </div>
      {error ? (
        <p role="alert" className="text-xs text-down">
          {error}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">{hint}</p>
      )}
    </div>
  );
}
