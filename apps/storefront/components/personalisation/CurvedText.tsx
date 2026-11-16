"use client";

// The buyer's text on a curved surface: a mug, bottle or candle. Each
// letter is placed on the cylinder by cylinderGlyphs, using the font's
// own advances as the browser measures them (kerning included), so it
// narrows as it turns away while the spacing stays the font's. The
// layout needs the print area's pixel size and the font's metrics, so
// the first render (and the server's) is the flat text, swapped once
// both are measured.

import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";

import { curvedTextLayout, cylinderGlyphs } from "@/lib/personalisation-text-fit";

type Measured = { starts: number[]; widths: number[] }[];

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
  const probe = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  const [measured, setMeasured] = useState<Measured | null>(null);
  const [fontsReady, setFontsReady] = useState(0);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => {
      const { width, height } = e!.contentRect;
      setSize(width > 0 && height > 0 ? { w: width, h: height } : null);
    });
    ro.observe(el);
    // A web font that arrives late changes the advances: measure again.
    void document.fonts?.ready.then(() => setFontsReady((n) => n + 1));
    return () => ro.disconnect();
  }, []);

  const layout = size ? curvedTextLayout(value, font, size.w, size.h) : null;
  const lines = value.split("\n");

  // Measure every letter's advance in the probe <text> elements.
  useLayoutEffect(() => {
    const svg = probe.current;
    if (!svg || !layout) return;
    const texts = [...svg.querySelectorAll("text")];
    setMeasured(
      texts.map((t, li) => {
        // One entry per code point, as the glyphs are rendered; the
        // substring offsets are UTF-16 units, so an emoji spans two.
        const starts: number[] = [];
        const widths: number[] = [];
        let at = 0;
        for (const ch of Array.from(lines[li] ?? "")) {
          starts.push(at ? t.getSubStringLength(0, at) : 0);
          widths.push(t.getSubStringLength(at, ch.length));
          at += ch.length;
        }
        return { starts, widths };
      }),
    );
    // layout?.fontSize covers a resize; value, font and fontsReady a re-measure.
  }, [value, font, layout?.fontSize, fontsReady]); // eslint-disable-line react-hooks/exhaustive-deps

  const textStyle = { fontFamily: style.fontFamily, fontWeight: style.fontWeight, fontStyle: style.fontStyle };
  const fill = (style.color as string) ?? "currentColor";

  return (
    <span ref={box} className="absolute inset-0 flex items-center justify-center">
      {layout && size ? (
        <>
          {/* Off-screen probe: the same text, unplaced, just to measure it. */}
          <svg ref={probe} aria-hidden="true" width="0" height="0" className="absolute overflow-hidden" style={{ visibility: "hidden" }}>
            {lines.map((l, i) => (
              <text key={i} fontSize={layout.fontSize} style={textStyle}>
                {l}
              </text>
            ))}
          </svg>
          {measured && measured.length === lines.length ? (
            <svg
              role="img"
              // The buyer's words, read as one phrase, not letter by letter.
              aria-label={value}
              width={size.w}
              height={size.h}
              viewBox={`0 0 ${size.w} ${size.h}`}
              // Not overflow-hidden: a long name's ends may run past the box
              // edge slightly, as they would round a real mug.
              className="overflow-visible"
              style={{ mixBlendMode: style.mixBlendMode, opacity: style.opacity, filter: style.filter }}
            >
              {lines.map((l, i) =>
                cylinderGlyphs(measured[i]!.starts, measured[i]!.widths, size.w, layout.baselines[i]!, layout.rise).map(
                  (g, j) => (
                    <text
                      key={`${i}-${j}`}
                      aria-hidden="true"
                      fontSize={layout.fontSize}
                      fill={fill}
                      textAnchor="middle"
                      style={textStyle}
                      transform={`translate(${g.x.toFixed(2)} ${g.y.toFixed(2)}) scale(${g.scaleX.toFixed(3)} 1)`}
                    >
                      {Array.from(l)[j]}
                    </text>
                  ),
                ),
              )}
            </svg>
          ) : (
            flat
          )}
        </>
      ) : (
        flat
      )}
    </span>
  );
}
