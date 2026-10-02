// Helpers for Kubernetes discovery: label commands and object names.

/** The label that opts a resource into discovery. */
export const MONITOR_LABEL = "upti.my/monitor";

/** Namespaces hidden by default in the resource browser. */
export const SYSTEM_NAMESPACES = new Set(["kube-system", "kube-public", "kube-node-lease"]);

/** The cluster's own plumbing: system namespaces and the API server's Service. */
export function isSystemResource(r: { kind: string; namespace: string; name: string }) {
  return (
    SYSTEM_NAMESPACES.has(r.namespace) || (r.kind === "service" && r.namespace === "default" && r.name === "kubernetes")
  );
}

/** "service/shop/checkout#shop.example.com" → "service shop/checkout". */
export function describeRef(ref: string) {
  const [object] = ref.split("#");
  const [kind, namespace, name] = object.split("/");
  return name ? `${kind} ${namespace}/${name}` : ref;
}

/**
 * kubectl commands that label (or, with stop, unlabel) resources, one line
 * per namespace and kind: `kubectl -n shop label service checkout cart upti.my/monitor=true`.
 */
export function labelCommands(resources: { kind: string; namespace: string; name: string }[], stop = false) {
  const groups = new Map<string, string[]>();
  for (const r of resources) {
    const key = `${r.namespace}\u0000${r.kind}`;
    groups.set(key, [...(groups.get(key) ?? []), r.name]);
  }
  const label = stop ? `${MONITOR_LABEL}-` : `${MONITOR_LABEL}=true`;
  return [...groups].map(([key, names]) => {
    const [namespace, kind] = key.split("\u0000");
    return `kubectl -n ${namespace} label ${kind} ${names.join(" ")} ${label}${stop ? "" : " --overwrite"}`;
  });
}
