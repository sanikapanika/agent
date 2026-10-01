import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

type Tip = { x: number; y: number; content: React.ReactNode };

/**
 * An instant tooltip for dense rows of small targets (uptime bars). The native
 * `title` tooltip waits about a second before appearing and can't be sped up.
 * The tip is portalled to <body> with fixed positioning so parent
 * `overflow: hidden` can't clip it, and nudged to stay inside the viewport.
 *
 * Usage: spread `bind(content)` onto each target and render `tip` once.
 */
export function useTooltip() {
  const [tip, setTip] = useState<Tip | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const [shift, setShift] = useState(0);

  const hide = useCallback(() => setTip(null), []);

  const bind = useCallback(
    (content: React.ReactNode) => {
      const show = (el: Element) => {
        const r = el.getBoundingClientRect();
        setTip({ x: r.left + r.width / 2, y: r.top, content });
      };
      return {
        onPointerEnter: (e: React.PointerEvent) => show(e.currentTarget),
        onPointerLeave: hide,
        // Touch has no hover: a tap shows the tip.
        onPointerDown: (e: React.PointerEvent) => e.pointerType !== "mouse" && show(e.currentTarget),
      };
    },
    [hide],
  );

  // Keep the tip on screen near the left/right edges.
  useLayoutEffect(() => {
    if (!tip || !ref.current) return;
    const w = ref.current.offsetWidth;
    const margin = 8;
    const left = tip.x - w / 2;
    const right = tip.x + w / 2;
    setShift(
      left < margin ? margin - left : right > window.innerWidth - margin ? window.innerWidth - margin - right : 0,
    );
  }, [tip]);

  // A fixed-position tip would drift away from its bar on scroll; hide it.
  useLayoutEffect(() => {
    if (!tip) return;
    window.addEventListener("scroll", hide, true);
    return () => window.removeEventListener("scroll", hide, true);
  }, [tip, hide]);

  const node = tip
    ? createPortal(
        <div
          ref={ref}
          role="tooltip"
          className="pointer-events-none fixed z-50 rounded-md bg-neutral-900 px-2.5 py-1.5 text-xs whitespace-nowrap text-white shadow-lg dark:bg-neutral-800"
          style={{ left: tip.x + shift, top: tip.y - 8, transform: "translate(-50%, -100%)" }}
        >
          {tip.content}
        </div>,
        document.body,
      )
    : null;

  return { bind, hide, tip: node };
}
