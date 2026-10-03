import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, KeyRound, ShieldCheck } from "lucide-react";
import { renderSVG } from "uqr";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { CopyButton, CopyField } from "@/components/ui/copy-button";
import { ErrorNote } from "@/components/Layout";
import { toast } from "@/components/ui/toast";

/** Two-factor sign-in for the signed-in user: set up, recovery codes, turn off. */
export function TwoFactorCard() {
  const qc = useQueryClient();
  const status = useQuery({ queryKey: ["two-factor"], queryFn: api.twoFactor });
  const [dialog, setDialog] = useState<"setup" | "codes" | "off" | null>(null);
  // One new secret per click: setting up again replaces the stored one, so
  // the QR code shown must be the one this call returned.
  const setup = useMutation({ mutationFn: api.setupTwoFactor, onSuccess: () => setDialog("setup") });
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["two-factor"] });
    qc.invalidateQueries({ queryKey: ["auth"] });
  };
  const on = status.data?.enabled ?? false;
  const left = status.data?.recovery_codes_left ?? 0;

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-start justify-between gap-4">
        <div>
          <CardTitle className="flex items-center gap-2">
            Two-factor sign-in {on && <Badge className="border-up/40 text-up">On</Badge>}
          </CardTitle>
          <CardDescription>
            {on
              ? `Signing in takes your password and a code from your authenticator app. ${left} recovery code${left === 1 ? "" : "s"} left.`
              : "Ask for a code from an authenticator app (1Password, Google Authenticator, Authy, ...) after your password. API tokens keep working."}
          </CardDescription>
        </div>
        {status.isSuccess &&
          (on ? (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => setDialog("codes")}>
                <KeyRound /> New recovery codes
              </Button>
              <Button variant="outline" size="sm" onClick={() => setDialog("off")}>
                Turn off
              </Button>
            </div>
          ) : (
            <Button size="sm" onClick={() => setup.mutate()} disabled={setup.isPending}>
              <ShieldCheck /> Set up
            </Button>
          ))}
      </CardHeader>
      {left > 0 && left <= 3 && (
        <CardContent>
          <p className="text-sm text-paused">
            Only {left} recovery code{left === 1 ? "" : "s"} left. Make new ones before you run out.
          </p>
        </CardContent>
      )}
      <ErrorNote error={status.error ?? setup.error} />
      {dialog === "setup" && setup.data && (
        <SetupDialog
          secret={setup.data.secret}
          uri={setup.data.uri}
          onClose={() => {
            setDialog(null);
            refresh();
          }}
        />
      )}
      {dialog === "codes" && (
        <PasswordDialog
          title="New recovery codes"
          action="Make new codes"
          note="Your current recovery codes stop working."
          run={(password) => api.newRecoveryCodes(password)}
          onClose={() => {
            setDialog(null);
            refresh();
          }}
        />
      )}
      {dialog === "off" && (
        <PasswordDialog
          title="Turn off two-factor sign-in"
          action="Turn off"
          note="Signing in will take only your password again. Your recovery codes stop working."
          run={async (password) => {
            await api.disableTwoFactor(password);
            toast.success("Two-factor sign-in is off");
            return null;
          }}
          onClose={() => {
            setDialog(null);
            refresh();
          }}
        />
      )}
    </Card>
  );
}

/** Scan, confirm with a code, then save the recovery codes. */
function SetupDialog({ secret, uri, onClose }: { secret: string; uri: string; onClose: () => void }) {
  const [code, setCode] = useState("");
  const enable = useMutation({ mutationFn: () => api.enableTwoFactor(code) });

  if (enable.data) {
    return (
      <Dialog title="Save your recovery codes" icon={KeyRound} onClose={onClose}>
        <div className="flex flex-col gap-4 border-y p-6">
          <RecoveryCodes codes={enable.data.recovery_codes} />
          <p className="text-sm text-muted-foreground">
            Two-factor sign-in is on, and your other devices were signed out.
          </p>
        </div>
        <div className="flex gap-3 p-6 pt-4">
          <Button className="flex-1" onClick={onClose}>
            I&apos;ve saved them
          </Button>
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog title="Set up two-factor sign-in" icon={ShieldCheck} onClose={onClose} busy={enable.isPending}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          enable.mutate();
        }}
      >
        <div className="flex flex-col gap-4 border-y p-6">
          <p className="text-sm text-muted-foreground">
            1. Scan this with your authenticator app, or enter the key by hand.
          </p>
          <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-start">
            <div
              className="size-40 shrink-0 rounded-lg bg-white p-2"
              aria-label="QR code for your authenticator app"
              role="img"
              // uqr renders an SVG of the otpauth:// URI; dark on white, so it scans in dark mode too.
              dangerouslySetInnerHTML={{ __html: renderSVG(uri, { border: 1 }) }}
            />
            <div className="w-full min-w-0">
              <p className="mb-1 text-xs text-muted-foreground">Key</p>
              <CopyField text={secret} label="Copy key">
                {secret.match(/.{1,4}/g)?.join(" ")}
              </CopyField>
            </div>
          </div>
          <Field label="2. Enter the 6-digit code it shows" htmlFor="totp-code">
            <Input
              id="totp-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
              autoFocus
              placeholder="123456"
              className="font-mono tracking-widest"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
          <ErrorNote error={enable.error} />
        </div>
        <div className="flex gap-3 p-6 pt-4">
          <Button type="button" variant="outline" className="flex-1" onClick={onClose} disabled={enable.isPending}>
            Cancel
          </Button>
          <Button type="submit" className="flex-1" disabled={enable.isPending}>
            Turn on
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

/** Confirms with the password, then runs the action; shows new codes if it returns any. */
function PasswordDialog({
  title,
  action,
  note,
  run,
  onClose,
}: {
  title: string;
  action: string;
  note: string;
  run: (password: string) => Promise<{ recovery_codes: string[] } | null>;
  onClose: () => void;
}) {
  const [password, setPassword] = useState("");
  const mutation = useMutation({
    mutationFn: () => run(password),
    onSuccess: (data) => {
      if (!data) onClose();
    },
  });
  if (mutation.data) {
    return (
      <Dialog title="Save your new recovery codes" icon={KeyRound} onClose={onClose}>
        <div className="border-y p-6">
          <RecoveryCodes codes={mutation.data.recovery_codes} />
        </div>
        <div className="flex gap-3 p-6 pt-4">
          <Button className="flex-1" onClick={onClose}>
            I&apos;ve saved them
          </Button>
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog title={title} onClose={onClose} busy={mutation.isPending}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
      >
        <div className="flex flex-col gap-4 border-y p-6">
          <p className="text-sm text-muted-foreground">{note}</p>
          <Field label="Your password" htmlFor="confirm-password">
            <Input
              id="confirm-password"
              type="password"
              autoComplete="current-password"
              required
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <ErrorNote error={mutation.error} />
        </div>
        <div className="flex gap-3 p-6 pt-4">
          <Button type="button" variant="outline" className="flex-1" onClick={onClose} disabled={mutation.isPending}>
            Cancel
          </Button>
          <Button type="submit" className="flex-1" disabled={mutation.isPending}>
            {action}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

/** The codes, once: each signs in one time when the phone is gone. */
function RecoveryCodes({ codes }: { codes: string[] }) {
  const text = codes.join("\n");
  const download = () => {
    const url = URL.createObjectURL(new Blob([text + "\n"], { type: "text/plain" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: "uptimy-agent-recovery-codes.txt" });
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <div>
      <p className="text-sm text-muted-foreground">
        Each code signs you in once if you lose your phone. Keep them somewhere safe, like a password manager. They
        won&apos;t be shown again.
      </p>
      <div className="mt-4 grid grid-cols-2 gap-x-6 gap-y-1.5 rounded-lg border bg-muted/40 p-4 font-mono text-sm">
        {codes.map((c) => (
          <span key={c}>{c}</span>
        ))}
      </div>
      <div className="mt-3 flex items-center gap-2">
        <CopyButton text={text} label="Copy recovery codes" />
        <Button variant="outline" size="sm" onClick={download}>
          <Download /> Download
        </Button>
      </div>
    </div>
  );
}
