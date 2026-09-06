# CT-Flow frontend

The company reference is `../middlefront` v0.4.0. CT-Flow implements its visual
language with existing React components and CSS, since Middlefront exports
Svelte 5 components and CT-Flow runs Next.js 15 / React 19.

## Reference mapping

| Middlefront source | CT-Flow implementation |
| --- | --- |
| `src/style/theme.css` | `frontend/app/globals.css`: neutral light/dark surfaces, semantic colors, Figtree + Noto Sans Thai, 6/10/14px radii, subtle shadows |
| `src/app/layout/shell/Shell.svelte` | `components/AppShell.tsx`: sidebar, topbar, navigation and account footer |
| `src/app/layout/login/variant.ts` | `entry/login/page.tsx`: responsive split brand/sign-in layout |
| `src/component/basis/button/variant.ts` | `.btn`: dark primary, outline, ghost and destructive variants |
| `src/component/basis/card/variant.ts` | `.card`: neutral surface, fine border, restrained elevation |

## Product decisions

- Default light theme; remember the user's light/dark choice in this browser.
- Teal is the CT-Flow accent, used for navigation, selected states and identity.
  Normal text uses darker teal in light mode and lighter teal in dark mode.
- Keep a 232px desktop sidebar; use wrapping navigation above content on mobile.
- Use real project counts. A project's total image count is not available on the
  summary endpoint, so cards do not invent completion percentages.
- Keep labeling images on a dark checkerboard in both themes.
- Retain existing SVG icons, API contracts and the detection module boundary.
- Do not place the unimplemented upload control in the project creation form.
- Keep visible focus, native modal semantics, autocomplete and reduced motion.
- External fonts use `display=swap` and system fallbacks for intranet installs.

The ui-ux-pro-max search supported a minimal enterprise style. Its generated
landing-page pattern did not match this application after a narrower retry;
Middlefront's actual Shell and Login patterns take precedence. Stack search did
not provide a verified responsive React match; responsive behavior follows the
skill's web layout guidance and is checked in a browser.

## Validation

Use `frontend/scripts/check-ui.mjs` with Playwright installed in a temporary
directory (instructions are at the top of the script). The script intercepts
all API calls and exercises real rendered components without changing server data.
It checks ownership filtering, searching, sorting, rename, cancellation, modal
focus restoration, theme persistence, API error recovery and workflow navigation
at desktop/mobile widths, and saves screenshots outside the repository.

Also run the repository's module boundary check, `npx tsc --noEmit` and
`npm run build` from `frontend/`. Mocked browser checks do not validate live
OAuth, model inference or production backend connectivity.
