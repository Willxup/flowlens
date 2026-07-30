import {
  cloneElement,
  useCallback,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactElement,
} from "react";
import { createPortal } from "react-dom";

interface TooltipProps {
  content: string;
  children: ReactElement<{ "aria-describedby"?: string }>;
  className?: string;
  placement?: "top" | "side";
}

interface TooltipPosition {
  left: number;
  top: number;
  ready: boolean;
}

export function Tooltip({
  content,
  children,
  className,
  placement = "top",
}: TooltipProps) {
  const id = useId();
  const anchorRef = useRef<HTMLSpanElement>(null);
  const tooltipRef = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<TooltipPosition>({
    left: 0,
    top: 0,
    ready: false,
  });

  const updatePosition = useCallback(() => {
    const anchor = anchorRef.current;
    const tooltip = tooltipRef.current;
    if (anchor === null || tooltip === null) return;

    const gutter = 10;
    const gap = 8;
    const anchorBox = anchor.getBoundingClientRect();
    const tooltipBox = tooltip.getBoundingClientRect();
    let left: number;
    let top: number;

    if (
      placement === "side" &&
      anchorBox.right + gap + tooltipBox.width <= window.innerWidth - gutter
    ) {
      left = anchorBox.right + gap;
      top = anchorBox.top + anchorBox.height / 2 - tooltipBox.height / 2;
    } else if (
      placement === "side" &&
      anchorBox.left - gap - tooltipBox.width >= gutter
    ) {
      left = anchorBox.left - gap - tooltipBox.width;
      top = anchorBox.top + anchorBox.height / 2 - tooltipBox.height / 2;
    } else {
      const centeredLeft =
        anchorBox.left + anchorBox.width / 2 - tooltipBox.width / 2;
      left = Math.min(
        Math.max(centeredLeft, gutter),
        Math.max(gutter, window.innerWidth - tooltipBox.width - gutter),
      );
      const above = anchorBox.top - tooltipBox.height - gap;
      top =
        above >= gutter
          ? above
          : Math.min(
              anchorBox.bottom + gap,
              window.innerHeight - tooltipBox.height - gutter,
            );
    }

    setPosition({
      left: Math.max(gutter, left),
      top: Math.min(
        Math.max(gutter, top),
        Math.max(gutter, window.innerHeight - tooltipBox.height - gutter),
      ),
      ready: true,
    });
  }, [placement]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [open, updatePosition]);

  function show() {
    setPosition((current) => ({ ...current, ready: false }));
    setOpen(true);
  }

  function hide() {
    setOpen(false);
  }

  return (
    <>
      <span
        className={["tooltip-anchor", className].filter(Boolean).join(" ")}
        ref={anchorRef}
        onPointerEnter={show}
        onPointerLeave={hide}
        onFocus={show}
        onBlur={hide}
        onKeyDown={(event) => {
          if (event.key === "Escape") hide();
        }}
      >
        {cloneElement(children, {
          "aria-describedby": open ? id : undefined,
        })}
      </span>
      {open
        ? createPortal(
            <span
              className="tooltip-popup"
              id={id}
              ref={tooltipRef}
              role="tooltip"
              style={{
                left: position.left,
                top: position.top,
                visibility: position.ready ? "visible" : "hidden",
              }}
            >
              {content}
            </span>,
            document.body,
          )
        : null}
    </>
  );
}

export function InfoTooltip({
  content,
  label,
}: {
  content: string;
  label: string;
}) {
  return (
    <Tooltip content={content} placement="side">
      <button className="info-tooltip-trigger" type="button" aria-label={label}>
        <svg viewBox="0 0 16 16" aria-hidden="true">
          <circle cx="8" cy="8" r="6.25" />
          <path d="M8 7.1v3.4M8 4.7h.01" />
        </svg>
      </button>
    </Tooltip>
  );
}
