"use client";

// The buyer's text on a curved surface: a mug, bottle or candle. Drawn as
// SVG along a curve so it wraps round the product the way the print will,
// while keeping the font's own letter spacing. The curve needs the print
// area's size in pixels, so the first render (and the server's) is the
// flat text, swapped once the box is measured.

import { useEffect, useId, useRef, useState, type CSSProperties, type ReactNode } from "react";

import { curvedTextLayout } from "@/lib/personalisation-text-fit";

export function CurvedText({
  value,
  font,
  style,
  flat,
}: {
  value: string;
  font?: string;
  /** Font family, colour and blend, shared with the flat preview. */
  style: CSSProperties;
  /** What to show until the print area has been measured. */
  flat: ReactNode;
}) {
  const box = useRef<HTMLSpanElement>(null);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  // useId's colons are not safe in an href fragment.
  const id = `curve${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => {
      const { width, height } = e!.contentRect;
      setSize(width > 0 && height > 0 ? { w: width, h: height } : null);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const layout = size ? curvedTextLayout(value, font, size.w, size.h) : null;

  return (
    <span ref={box} className="absolute inset-0 flex items-center justify-center">
      {layout && size ? (
        <svg
          width={size.w}
          height={size.h}
          viewBox={`0 0 ${size.w} ${size.h}`}
          // Not overflow-hidden: a long name's ends may run past the box
          // edge slightly, as they would round a real mug.
          className="overflow-visible"
          style={{ mixBlendMode: style.mixBlendMode, opacity: style.opacity, filter: style.filter }}
        >
          {/* Real text, so a screen reader reads the buyer's words. */}
          <text
            fontSize={layout.fontSize}
            fill={(style.color as string) ?? "currentColor"}
            style={{ fontFamily: style.fontFamily, fontWeight: style.fontWeight, fontStyle: style.fontStyle }}
          >
            {layout.lines.map((_, i) => (
              <textPath key={i} href={`#${id}-${i}`} startOffset="50%" textAnchor="middle">
                {value.split("\n")[i]}
              </textPath>
            ))}
          </text>
          <defs>
            {layout.lines.map((d, i) => (
              <path key={i} id={`${id}-${i}`} d={d} />
            ))}
          </defs>
        </svg>
      ) : (
        flat
      )}
    </span>
  );
}
