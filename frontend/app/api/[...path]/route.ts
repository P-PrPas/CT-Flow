// Runtime proxy to FastAPI. next.config rewrites are baked in at build time,
// so they can't read API_URL from the container env -- this can.
const API = process.env.API_URL ?? "http://127.0.0.1:8000";

/** Forwarded upstream: the content type (multipart uploads carry their
 *  boundary in it, so it cannot be rewritten) and the session cookie. */
const TO_API = ["content-type", "cookie"];
/** Forwarded back: what the browser needs to render or store the response. */
const FROM_API = ["content-type", "content-disposition"];

async function proxy(req: Request, path: string[]) {
  const url = new URL(req.url);
  const headers = new Headers();
  for (const h of TO_API) {
    const v = req.headers.get(h);
    if (v) headers.set(h, v);
  }

  const res = await fetch(`${API}/api/${path.join("/")}${url.search}`, {
    method: req.method,
    headers,
    // arrayBuffer, not text: uploads are bytes, and text() would mangle them.
    body: req.method === "GET" || req.method === "HEAD" ? undefined : await req.arrayBuffer(),
  });

  const out = new Headers();
  for (const h of FROM_API) {
    const v = res.headers.get(h);
    if (v) out.set(h, v);
  }
  if (!out.has("content-type")) out.set("content-type", "application/json");
  // Login/logout set a cookie; without this the session never reaches the browser.
  for (const c of res.headers.getSetCookie?.() ?? []) out.append("set-cookie", c);

  return new Response(res.body, { status: res.status, headers: out });
}

type Ctx = { params: Promise<{ path: string[] }> };
const handler = async (req: Request, ctx: Ctx) => proxy(req, (await ctx.params).path);

// Every method the client uses. DELETE was missing, which turned
// "clear evaluate history" into a silent 405.
export const GET = handler;
export const POST = handler;
export const PUT = handler;
export const PATCH = handler;
export const DELETE = handler;
