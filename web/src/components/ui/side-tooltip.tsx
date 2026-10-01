import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

/**
 * A tooltip to the right of its trigger, for the collapsed sidebar rail (like
 * the app's Tooltip with placement "right" and a 200ms open delay). Portalled
 * with fixed positioning so the sidebar's overflow can't clip it.
 */
export function SideTooltip({
  content,
  disabled,
  children,
}: {
  content: React.ReactNode;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const timer = useRef<number | undefined>(undefined);
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);

  const show = () => {
    if (disabled) return;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      const r = ref.current?.getBoundingClientRect();
      if (r) setPos({ x: r.right + 12, y: r.top + r.height / 2 });
    }, 200);
  };
  const hide = () => {
    window.clearTimeout(timer.current);
    setPos(null);
  };

  useEffect(() => () => window.clearTimeout(timer.current), []);
  // Fixed positioning would leave the tip behind on scroll; hide it instead.
  useEffect(() => {
    if (!pos) return;
    window.addEventListener("scroll", hide, true);
    return () => window.removeEventListener("scroll", hide, true);
  }, [pos]);

  return (
    <div ref={ref} onPointerEnter={show} onPointerLeave={hide} onFocus={show} onBlur={hide} onClick={hide}>
      {children}
      {pos &&
        !disabled &&
        createPortal(
          <div
            role="tooltip"
            className="pointer-events-none fixed z-50 -translate-y-1/2 rounded-md bg-neutral-900 px-2.5 py-1.5 text-xs whitespace-nowrap text-white shadow-lg dark:bg-neutral-800"
            style={{ left: pos.x, top: pos.y }}
          >
            {content}
          </div>,
          document.body,
        )}
    </div>
  );
}
