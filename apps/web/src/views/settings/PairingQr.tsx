import qrcode from "qrcode-generator";
import { createMemo } from "solid-js";

const QUIET_ZONE = 4;

/**
 * A QR code for some text, drawn as one SVG path. It is always dark on white whatever the theme,
 * because a phone's camera reads contrast, not a palette.
 */
export function PairingQr(props: { text: string; label: string }) {
  const drawn = createMemo(() => {
    const code = qrcode(0, "M");
    code.addData(props.text);
    code.make();
    const count = code.getModuleCount();
    let path = "";
    for (let row = 0; row < count; row += 1) {
      for (let col = 0; col < count; col += 1) {
        if (code.isDark(row, col)) path += `M${col + QUIET_ZONE} ${row + QUIET_ZONE}h1v1h-1z`;
      }
    }
    return { path, size: count + QUIET_ZONE * 2 };
  });
  return (
    <svg
      role="img"
      aria-label={props.label}
      viewBox={`0 0 ${drawn().size} ${drawn().size}`}
      width={144}
      height={144}
      shape-rendering="crispEdges"
      class="rounded-sm"
    >
      <rect width="100%" height="100%" fill="#fff" />
      <path d={drawn().path} fill="#000" />
    </svg>
  );
}
