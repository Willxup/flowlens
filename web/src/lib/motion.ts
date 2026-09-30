import { useEffect, useLayoutEffect, useRef, useState } from "react";

export const reducedMotion = () =>
  window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

// Keep keyed rows visually attached to their data when rankings change.
export function useReorderMotion<T extends HTMLElement>(
  selector: string,
  order: string,
) {
  const ref = useRef<T>(null);
  const previous = useRef(new Map<Element, { x: number; y: number }>());
  const animations = useRef(new Map<Element, Animation>());
  useLayoutEffect(() => {
    const elements = [
      ...(ref.current?.querySelectorAll<HTMLElement>(selector) ?? []),
    ];
    const next = new Map<Element, { x: number; y: number }>();
    for (const element of elements) {
      const old = previous.current.get(element);
      const running = animations.current.get(element);
      const wasRunning = running?.playState === "running";
      const visual = element.getBoundingClientRect();
      running?.cancel();
      const box = element.getBoundingClientRect();
      const position = {
        x: box.left + window.scrollX,
        y: box.top + window.scrollY,
      };
      next.set(element, position);
      if (!old || !element.animate || reducedMotion()) continue;
      const x = old.x - position.x + (wasRunning ? visual.left - box.left : 0);
      const y = old.y - position.y + (wasRunning ? visual.top - box.top : 0);
      if (Math.abs(x) + Math.abs(y) < 1) continue;
      const animation = element.animate(
        [
          { transform: `translate(${x}px, ${y}px)` },
          { transform: "translate(0, 0)" },
        ],
        { duration: 220, easing: "cubic-bezier(.2,.75,.2,1)" },
      );
      animations.current.set(element, animation);
      animation.onfinish = () => animations.current.delete(element);
    }
    for (const [element, animation] of animations.current) {
      if (!next.has(element)) {
        animation.cancel();
        animations.current.delete(element);
      }
    }
    previous.current = next;
  }, [selector, order]);
  useEffect(
    () => () => {
      for (const animation of animations.current.values()) animation.cancel();
    },
    [],
  );
  return ref;
}

// Animate only the selected background, never delay the actual selection.
export function useSelectionMotion() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    const states = new Map<
      HTMLElement,
      {
        indicator: HTMLSpanElement;
        selected: Element;
        x: number;
        y: number;
        width: number;
        height: number;
        animation?: Animation;
      }
    >();
    let frame = 0;
    const update = () => {
      for (const [group, state] of states) {
        if (!root.contains(group)) {
          state.animation?.cancel();
          states.delete(group);
        }
      }
      root
        .querySelectorAll<HTMLElement>(
          ".side-nav, .mobile-nav, .segmented, .dimension-tabs, .chart-toggle, .sort-control",
        )
        .forEach((group) => {
          const selected = group.querySelector<HTMLElement>(
            'button[aria-current="page"], button[aria-pressed="true"]',
          );
          if (!selected || group.getBoundingClientRect().width === 0) {
            const state = states.get(group);
            if (state) state.indicator.hidden = true;
            return;
          }
          const existing = states.get(group);
          if (existing) existing.indicator.hidden = false;
          const box = selected.getBoundingClientRect();
          const parent = group.getBoundingClientRect();
          const x = box.left - parent.left - group.clientLeft;
          const y = box.top - parent.top - group.clientTop;
          const old = states.get(group);
          if (
            old &&
            old.selected === selected &&
            old.x === x &&
            old.y === y &&
            old.width === box.width &&
            old.height === box.height
          )
            return;
          const indicator = old?.indicator ?? document.createElement("span");
          if (!old) {
            indicator.className = "selection-indicator";
            indicator.setAttribute("aria-hidden", "true");
            group.classList.add("motion-selection");
            group.prepend(indicator);
          }
          const visual = old?.indicator.getBoundingClientRect();
          old?.animation?.cancel();
          Object.assign(indicator.style, {
            left: `${x}px`,
            top: `${y}px`,
            width: `${box.width}px`,
            height: `${box.height}px`,
          });
          const animation =
            old &&
            visual &&
            typeof indicator.animate === "function" &&
            !reducedMotion()
              ? indicator.animate(
                  [
                    {
                      transform: `translate(${visual.left - box.left}px, ${visual.top - box.top}px) scale(${visual.width / box.width}, ${visual.height / box.height})`,
                    },
                    { transform: "translate(0, 0) scale(1, 1)" },
                  ],
                  { duration: 180, easing: "cubic-bezier(.2,.75,.2,1)" },
                )
              : undefined;
          states.set(group, {
            indicator,
            selected,
            x,
            y,
            width: box.width,
            height: box.height,
            animation,
          });
        });
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    const groups =
      ".side-nav, .mobile-nav, .segmented, .dimension-tabs, .chart-toggle, .sort-control";
    const observer = new MutationObserver((records) => {
      if (
        records.some(
          (record) =>
            record.type === "attributes" ||
            [...record.addedNodes, ...record.removedNodes].some(
              (node) =>
                node instanceof HTMLElement &&
                (node.matches(groups) || node.querySelector(groups)),
            ),
        )
      )
        schedule();
    });
    observer.observe(root, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["aria-pressed", "aria-current"],
    });
    const resize = new ResizeObserver(schedule);
    resize.observe(root);
    window.addEventListener("resize", schedule);
    update();
    return () => {
      observer.disconnect();
      resize.disconnect();
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", schedule);
      for (const [group, state] of states) {
        state.animation?.cancel();
        state.indicator.remove();
        group.classList.remove("motion-selection");
      }
    };
  }, []);
  return ref;
}

export function useAnimatedClose(close: () => void) {
  const [closing, setClosing] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const callback = useRef(close);
  callback.current = close;
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );
  return {
    closing,
    close: () => {
      if (closing) return;
      if (reducedMotion()) {
        callback.current();
        return;
      }
      setClosing(true);
      timer.current = setTimeout(() => {
        setClosing(false);
        callback.current();
      }, 120);
    },
    cancel: () => {
      if (timer.current) clearTimeout(timer.current);
      setClosing(false);
    },
  };
}
