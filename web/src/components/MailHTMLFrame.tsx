import { useCallback, useMemo, useRef } from "react";

const framePolicy = [
  "default-src 'none'",
  "style-src 'unsafe-inline'",
  "img-src data:",
  "font-src 'none'",
  "connect-src 'none'",
  "media-src 'none'",
  "form-action 'none'",
  "base-uri 'none'",
].join("; ");

export default function MailHTMLFrame({ html }: { html: string }) {
  const frameRef = useRef<HTMLIFrameElement>(null);
  const srcDoc = useMemo(
    () => `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="${framePolicy}"><base target="_blank"><style>html{color-scheme:light}body{margin:0;padding:20px;color:#17211c;background:#fff;font:14px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",sans-serif;overflow-wrap:anywhere}img,table{max-width:100%}table{border-collapse:collapse}pre{white-space:pre-wrap}a{color:#176b4a}</style></head><body>${html}</body></html>`,
    [html],
  );
  const resize = useCallback(() => {
    const frame = frameRef.current;
    const height = frame?.contentDocument?.documentElement.scrollHeight;
    if (frame && height) frame.style.height = `${Math.min(Math.max(height, 180), 1200)}px`;
  }, []);

  return (
    <iframe
      ref={frameRef}
      className="mail-html-frame"
      title="邮件正文"
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      srcDoc={srcDoc}
      onLoad={resize}
    />
  );
}
