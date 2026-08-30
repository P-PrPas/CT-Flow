# CT-Flow — Gallery & pool scaling

> **สถานะ: implement แล้วบน `feature/gallery` (T-36–T-39)** — 2026-08-30
>
> T-36 ลงเป็น `GET /api/pool` อย่างเดียว · การตัด `images[]` ออกจาก `POST /api/session`
> **ถูกเลื่อน** เป็น T-36b: มันแตะ ~30 assertion ใน `smoke_test.py` และ ~10 จุดใน
> `session.ts` (ซึ่งไม่มี test รัน) โดยที่ปัญหา render/scroll ที่เป็นตัวเจ็บจริงถูกแก้หมดแล้วด้วย
> T-37–T-39 — ค่าที่เหลือคือ payload ~6MB ครั้งเดียวตอนเปิด session ซึ่งเป็น ceiling ที่บันทึกไว้
> **เอกสารที่เกี่ยวข้อง:** [ARCHITECTURE.md](./ARCHITECTURE.md) · [API_REFERENCE.md](./API_REFERENCE.md) · [REQUIREMENTS.md](./REQUIREMENTS.md) · [ROADMAP.md](./ROADMAP.md) · [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md)

---

## 1. โจทย์ (สภาพก่อนงานนี้)

ภาพที่ label แล้ว (มือหรือ auto) เก็บเข้า PostgreSQL ครบ แต่ **ไม่มีทางกลับไปดู/แก้เป็นชุด** ที่ใช้งานได้จริงเมื่อ dataset โต

ปัจจุบันมีที่เดียว: การ์ด **Queue** ใน sidebar ของแท็บ Pool (`PoolPanel.tsx`) render `sortedPool.map()` = `<div class="thumb-row">` หนึ่งอันต่อหนึ่งภาพ **ทั้ง `s.images`** และ `s.images` มาจาก `POST /api/session` ที่ตอบ `images.List(dir)` = ทุก path ส่งมาหมดตอนเปิด session

กับ dataset หลักหมื่นภาพ (`sample_50k` = 50,000) มี **4 กำแพง scaling** ซ้อนกัน สามในสี่เป็นปัญหา performance ไม่ใช่แค่ UX:

| # | กำแพง | เกิดอะไรกับ 50,000 ภาพ |
|---|---|---|
| 1 | **DOM / render** | 50k `<div>` + `<img loading="lazy">` · `loading="lazy"` เลื่อน network fetch ออกไป แต่ browser ยังสร้าง element + layout ครบ 50k · ทุกครั้งที่ save (labeled/auto เปลี่ยน) หรือ re-check (scores เปลี่ยน) React reconcile 50k node — นี่คือ jank ที่รู้สึกได้ ไม่ใช่แค่ scroll ยาว |
| 2 | **ไม่มี thumbnail endpoint** | `imgUrl(p)` → `GET /api/image` → `http.ServeFile` **full-res** · conveyor JPEG ~100–300KB/ภาพ · gallery grid หนึ่งจอ = 60 ภาพ = 6–18MB + decode เต็ม 60 รูป · เลื่อนไม่กี่จอบน 50k = ผลัก DB หลายร้อย MB, decoder ไหม้ — **gallery รันบน endpoint นี้ไม่ได้** |
| 3 | **Payload** | `POST /api/session` ตอบ `images[]` = 50k × path ~110 ตัวอักษร ≈ **6MB JSON** ทุกครั้งที่เปิด/รีเฟรช และบังคับให้ filter ทุกอย่างเป็น client-side |
| 4 | **UX** | ภาพ done ต่อท้ายคิวเดียวกับ to-do เรียงไปล่างสุด · status เป็น text (`labeled by hand` / `labeled by model`) ต้องอ่านเอง · อยู่ใน scroll box 46vh ใน sidebar แคบที่แชร์กับ canvas · ไม่มี filter / search / bulk |

**ข้อสังเกต:** การ์ด Queue ทำสองงานที่ไม่เกี่ยวกัน — (ก) "ภาพไหนควร label ต่อ" (FR-18 least-confident-first + spread) ต้องเก็บไว้ แต่ต้องการแค่ ~20–50 อันบนสุด · (ข) "ขอดู/ตรวจภาพที่ทำไปแล้วทั้งหมด" คืองานของ gallery และไม่ควรอยู่ใน sidebar ตอน label เลย

**งานนี้แก้:** แยกสองงานนั้นออกจากกัน + ทำ gallery ที่ scale ถึงหลักแสนภาพ **ไม่แก้อย่างอื่น**

---

## 2. สิ่งที่ตัดสินใจแล้ว (และเหตุผล)

| # | ตัดสินใจ | เหตุผลสั้น ๆ |
|---|---|---|
| 1 | **Pool listing ย้ายไป server-side** (`GET /api/pool`, paginate + filter) · `POST /api/session` เลิกส่ง `images[]` **(เลื่อนเป็น T-36b)** | ส่ง 50k path ทุกครั้งที่เปิด session คือ 6MB ที่ client เอาไปทำ filter เองไม่ไหวในระดับ DOM · server มี `images.status` ใน Postgres อยู่แล้ว, filter/slice ที่นั่นถูกที่ · endpoint ลงก่อน, การถอด `images[]` ออกจาก session ตามทีหลัง (แตะ contract กว้าง, `session.ts` ไม่มี test) |
| 2 | **Thumbnail endpoint** (`GET /api/thumb?path=&w=`) ย่อภาพ + cache | gallery grid บน full-res image เป็นไปไม่ได้ (กำแพง #2) · `golang.org/x/image` เป็น direct dep อยู่แล้ว → `x/image/draw` ฟรี ไม่เพิ่ม dependency |
| 3 | **Gallery เป็น panel แยก** (`"gallery"` ใน `Panel` union) ไม่ใช่การขยาย Queue card | Queue card คือ "label อะไรต่อ" · gallery คือ "ตรวจงานที่ทำไปแล้ว" · สอง mental model, สอง viewport, สอง data query |
| 4 | **Virtualization ทำด้วย CSS `content-visibility: auto`** ไม่เพิ่ม lib (`react-window` ฯลฯ) | house style: ไม่มี UI/virtualization library · cell ขนาดคงที่ + `contain-intrinsic-size` → browser ข้าม layout/paint ของ card นอกจอเอง native · 50k card ไหว |
| 5 | **Infinite scroll ด้วย `IntersectionObserver`** ไม่ทำ pagination UI (ปุ่มหน้า 1/2/3) | browser-native, ไม่มี state ให้พลาด · sentinel ท้าย list → fetch `offset` ถัดไป |
| 6 | **คลิก card → เปิดใน editor เดิม** (`goToImage` + สลับไป panel `pool`) ไม่ทำ editor ใหม่ใน gallery | editor ทั้งหมดมีอยู่แล้วและรองรับทั้ง manual (re-teach) และ auto (review mode) · gallery แค่เป็นทางเข้าที่ดีกว่า scroll |
| 7 | **Thumbnail cache เริ่มที่ in-process LRU** ไม่ทำ disk cache | `.ctflow/_thumbs/` เป็น dir ใหม่ที่ Go ต้องเป็นเจ้าของ · เริ่มด้วย LRU ก่อน, เพิ่ม disk เฉพาะเมื่อวัดแล้ว cold-scroll pool ใหญ่ช้าจริง |
| 8 | **conf/score ยังเป็น client-side** ไม่ fold เข้า `/api/pool` | `scores` วันนี้มาจาก `POST /api/score` และเป็น client-only · gallery เรียงตาม status/ชื่อพอ, ไม่ต้องเรียงตาม confidence · fold เข้าทีหลังถ้าจำเป็น |

**สิ่งที่งานนี้ตั้งใจไม่ทำ:** bulk operations (multi-select, bulk delete/re-teach/re-auto) · disk thumbnail cache · เรียง gallery ตาม confidence · thumbnail แบบ precompute ตอน ingest · filter ตามคลาส · gallery ของ test set (คนละ index space, ทำแยกถ้าจำเป็น)

---

## 3. API ที่เพิ่ม

รูปแบบ error, path safety และ auth ใช้ convention เดิมทุกข้อ (ดู [API_REFERENCE.md](./API_REFERENCE.md))

### 3.1 `GET /api/pool` — `internal/transport/httpapi/pool.go`

listing ของ pool แบบ paginate + filter สำหรับ gallery และ (ทีหลัง) Queue card

- **Query:** `input_dir` (required) · `status` = `all` (default) `| labeled | auto | unlabeled` · `offset` (default `0`) · `limit` (default `200`, cap `500`) · `order` = `name` (default) `| status`
- **Response:**
  ```json
  {
    "total": 50000,
    "counts": { "labeled": 120, "auto": 4300, "unlabeled": 45580 },
    "items": [
      { "path": "/opt/mount/project/ds/img_1.jpg", "status": "labeled", "held_by": null }
    ]
  }
  ```
- `counts` มาจาก SQL count ล้วน (`images.status` group by) + `total - labeled - auto` สำหรับ `unlabeled` — **ไม่อ่านโฟลเดอร์เพื่อนับ** (กติกาเดียวกับ `GET /api/projects` §4.1 ของ PHASE2_WORKSPACE)
- `total` = จำนวนไฟล์ภาพในโฟลเดอร์ (`len(images.List(dir))`) · `unlabeled` = `total - labeled - auto` (ภาพที่ไม่มีแถวใน `images` เลย)
- `items` เรียงตาม `order` แล้ว slice ด้วย `offset`/`limit` **ที่ server**
- `held_by` = username จาก claims map (nil ถ้าว่าง) — เหมือน `GET /api/state`
- `input_dir` ผ่าน `checkedPath()` เหมือนทุก endpoint ที่รับ path (invariant #8)
- **400** ถ้าโฟลเดอร์ไม่มีอยู่จริงหรือไม่มีภาพ · **404** ถ้าไม่มี project (`store.ErrNoProject`)

**`images.List(dir)` cache:** readdir 50k ไฟล์ทุก request ไม่ไหว — cache ผลต่อ `dir` พร้อม mtime ของโฟลเดอร์, invalidate เมื่อ mtime เปลี่ยน

```go
// ponytail: in-process map keyed by dir, guarded by sync.Mutex, invalidated on
// folder mtime change. Same "one API process" ceiling as jobs/claims -- move it
// out when they move (T-15).
```

### 3.2 `POST /api/session` — **ยังไม่เปลี่ยน (T-36b)**

ตั้งใจปล่อยให้ `POST /api/session` ยังส่ง `images[]` เต็มไปก่อน:

- `session.ts` ใช้ `images[]` ใน ~10 จุด — `remaining` (ส่งให้ autolabel/score), `sortedPool` (FR-18 spread), `nextTodo`, `poolCandidates` (test-set sampling), `saveReview`/`teachFromReview` (หาภาพ auto ถัดไป), keyboard nav — และ frontend ไม่มี test รัน
- `smoke_test.py` อ่าน `images = r.json()["images"]` แล้ว index (`images[0]`, `images[1:]`) ในราว 30 assertion
- ปัญหาที่เจ็บจริง (DOM 50k row, full-res image, scroll ยาว) ถูกแก้หมดแล้วโดย T-37–T-39 · ที่เหลือคือ payload ~6MB ครั้งเดียวตอนเปิด session

T-36b: เปลี่ยน `/api/autolabel` + `/api/score` ให้ enumerate ภาพที่ยังไม่ label ที่ server, แทน `images[]` ด้วย `pool: {total, counts}`, ย้าย `sortedPool`/`nextTodo` ไปดึงจาก `/api/pool` — งานของ PR แยก

### 3.3 `GET /api/thumb` — `internal/transport/httpapi/thumb.go`

```
GET /api/thumb?path=<abs path>&w=200
→ 200 image/jpeg (ย่อแล้ว) · Cache-Control: private, max-age=31536000, immutable · ETag: "<mtime>-<size>-<w>"
```

- `path` ผ่าน `checkedPath()` (invariant #8) · **404** ถ้าไฟล์หาย · **400** ถ้า decode ไม่ได้ (gallery ข้าม cell ที่พัง ไม่ทำทั้ง request ล้ม)
- `w` = ความกว้างเป้าหมาย, min 16, cap ที่ `400` (gallery ใช้ 200, retina ใช้ 400) · สูงคำนวณตาม aspect ratio · **ไม่ upscale** — source ที่เล็กกว่า `w` เสิร์ฟขนาดตัวเอง
- decode ด้วย stdlib `image` (decoder จาก blank import ใน `imagecheck.go`: jpeg/png/gif/bmp) → scale ด้วย `golang.org/x/image/draw` (`draw.ApproxBiLinear`) → encode JPEG q75
- ถ้า `If-None-Match` ตรง ETag → **304** ไม่ decode ใหม่
- **cache:** in-process LRU 512 entry (`container/list`), key = `path|w`

```go
// ponytail: in-memory LRU, 512 entries (~8MB at 200px). A cold scroll past the
// tail of a 50k pool re-decodes; add a .ctflow/_thumbs/ disk cache
// (Go-owned, sha1(path)_w.jpg) only if that measurably drags.
```

- route: `mux.Handle("GET /api/thumb", s.Handle(s.GetThumb))` ใน `backend/cmd/api/main.go`
- `app/api/[...path]/route.ts` (proxy) forward `cache-control`+`etag` กลับ และ `if-none-match` ขึ้น — ไม่งั้น browser cache ไม่ทำงานผ่าน proxy
- frontend: `thumbUrl(path, w = 200)` ใน `app/lib/api.ts` (ของกลาง — re-export ผ่าน `modules/detection/api.ts` เหมือน `imgUrl`)

### 3.4 `images.ListCached` — `internal/infra/images/images.go`

`GET /api/pool` เรียก `images.ListCached(dir)` แทน `List(dir)` — cache ต่อ dir keyed บน mtime ของโฟลเดอร์ (เปลี่ยนเมื่อ entry ถูกเพิ่ม/ลบ = เมื่อชุดภาพเปลี่ยน) ป้องกัน readdir 50k ไฟล์ทุก scroll page

```go
// ponytail: in-process map + sync.Mutex, one API process. Same "no horizontal
// scale" ceiling as the job and claim trackers -- when they move, this moves.
```

---

## 4. Frontend

### 4.1 โครง

```
app/modules/detection/
  panels/GalleryPanel.tsx        ใหม่ (state อยู่ใน panel เอง, ไม่ยัดเข้า session.ts)
  session.ts                     Panel union += "gallery" เท่านั้น
  api.ts                         getPool() + PoolItem · re-export thumbUrl
  panels/PoolPanel.tsx           Queue card render .slice(0, QUEUE_CAP) + ลิงก์ไป gallery
app/lib/api.ts                   thumbUrl()  (ของกลาง)
app/api/[...path]/route.ts       forward cache-control/etag/if-none-match
app/p/[id]/page.tsx              แท็บ "Gallery" ในแถวแท็บ
app/globals.css                  .gallery-grid / .gallery-cell / .gallery-dot / ...
```

`GalleryPanel` อยู่ใต้ `modules/detection/` — ห้ามถูก import จาก `app/page.tsx` (invariant #11)

### 4.2 GalleryPanel

- **แถบ filter:** chip `All 50000` · `By hand 120` · `By model 4300` · `Unlabeled 45580` — ตัวเลขจาก `counts`, คลิกเปลี่ยน `status` แล้ว refetch จาก `offset=0`
- **Grid:** CSS `display: grid; grid-template-columns: repeat(auto-fill, 150px)` · แต่ละ card:
  ```
  <button class="gallery-cell" onClick={() => open(path)}>
    <img src={thumbUrl(path, 200)} loading="lazy" width={150} height={112} />
    <span class="status-dot" data-status={status} />   {/* เขียว=hand, brand=model, เทา=unlabeled */}
    {held_by && <span class="held">{held_by}</span>}
  </button>
  ```
- **Native virtualization:** `.gallery-cell { content-visibility: auto; contain-intrinsic-size: 150px 150px; }` — browser ข้าม render ของ cell นอก viewport เอง ไม่ต้องคำนวณ window
- **Infinite scroll:** `<div ref={sentinel} />` ท้าย grid + `IntersectionObserver` → `offset += limit`, append `items`
- **คลิก card:** `s.goToImage(path); s.setPanel("pool")` — เปิดใน editor เดิม (manual → re-teach, auto → review mode ตาม §PoolPanel เดิม)
- **สถานะว่าง:** filter ที่ count = 0 → `<Empty>` ไม่ยิง fetch

### 4.3 การ์ด Queue ที่หดแล้ว (`PoolPanel.tsx`)

- render `s.sortedPool.slice(0, QUEUE_CAP)` (`QUEUE_CAP = 60`) แทนทั้งลิสต์ — `sortedPool` ยังคำนวณเต็ม (todo + done) เพื่อให้ keyboard next/prev เดินได้ทั้งโฟลเดอร์ แต่ DOM มีแค่ 60 row
- footer: ปุ่ม `See all N in the gallery` → `s.setPanel("gallery")` เมื่อ `sortedPool.length > QUEUE_CAP`

> **หมายเหตุ:** แผนเดิมบอก "ลบ done-list logic ออกจาก `sortedPool`" — เลื่อนไปพร้อม T-36b เพราะ keyboard nav (`step()` ใน `page.tsx`) ยังเดินผ่าน `sortedPool` ทั้งชุด การ slice ตอน render แก้ปัญหา DOM ได้โดยไม่แตะ nav

---

## 5. งานเป็นก้อน — สิ่งที่ลงจริงบน `feature/gallery`

| ก้อน | สถานะ | ไฟล์ |
|---|---|---|
| **T-36** | `GET /api/pool` ลงแล้ว · `POST /api/session` **ยังส่ง `images[]`** (→ T-36b) | `pool.go` (`GetPool`, `poolItem`), `images.go` (`ListCached`), `main.go` route, `api.ts` (`getPool`, `PoolItem`) |
| **T-37** | ลงครบ | `thumb.go` (`GetThumb` + LRU + ETag/304), `main.go` route, `lib/api.ts` (`thumbUrl`), `route.ts` (header forwarding) |
| **T-38** | ลงครบ | `panels/GalleryPanel.tsx` ใหม่, `session.ts` (`Panel` += `"gallery"`), `page.tsx` (แท็บ + render), `globals.css` (`.gallery-*`) |
| **T-39** | ลง version slice — ดู §4.3 | `panels/PoolPanel.tsx` |
| **T-36b** | ยังไม่ทำ — PR แยก | `/api/autolabel` + `/api/score` server-enumerate, ถอด `images[]` จาก session, ย้าย `sortedPool`/`nextTodo` ไป `/api/pool` |

**smoke_test.py** (`backend/tests/smoke_test.py`): เพิ่ม block สำหรับ `GET /api/pool` (fresh state, pagination, status filter, bad-status → 400, counts เปลี่ยนหลัง label) และ `GET /api/thumb` (image/jpeg เล็กกว่า original, `If-None-Match` → 304, path นอก root → 403)

**Go tests** (`httpapi_test.go`): `TestGetThumbScalesAndRevalidates` (decode ผลลัพธ์ เช็ค 120×80 + ETag + 304), `TestGetThumbNeverUpscales`, `TestGetThumbOnGarbageIs400`, และเพิ่ม `/api/thumb` + `/api/pool` เข้า `TestPathHandlersRefuseOutsideTheRoot`

---

## 6. Requirements ที่ต้องเพิ่มใน [REQUIREMENTS.md](./REQUIREMENTS.md)

ล่าสุดคือ FR-51 · เสนอ:

- **FR-52** — ผู้ใช้ดูภาพทั้งหมดในโปรเจกต์เป็น gallery แยกจากคิว label, filter ตามสถานะ (มือ / โมเดล / ยังไม่ทำ), คลิกเปิดแก้ได้
- **FR-53** — ระบบ serve thumbnail ย่อสำหรับ gallery ไม่ส่ง full-res
- **FR-54** — pool listing paginate ที่ server (T-36b: `POST /api/session` ไม่ส่งรายการ path ทั้งหมด)

และ [ROADMAP.md](./ROADMAP.md): งานนี้เป็น **Phase 4 (UX polish)** ไม่ใช่ Phase 5 (scale/ops) — มันคือ UX ที่จำเป็นต่อผู้ใช้เมื่อ dataset โต ไม่ใช่ horizontal scale · แต่ `images.ListCached` กับ thumbnail LRU มี ceiling "process เดียว" ข้อเดียวกับ jobs/claims — บันทึกในตาราง "ข้อจำกัดที่รู้ตัว"

---

## 7. Testing

```bash
cd backend && go test ./...
SMOKE_BASE_URL=http://localhost:8000 python -m backend.tests.smoke_test
cd frontend && npx tsc --noEmit && npm run build
```

`backend/tests/smoke_test.py` **คือ contract** — endpoint หรือ behaviour ที่เปลี่ยนทุกอย่างต้องมี assertion ที่นั่น เมื่อ T-36b ตัด `images` ออกจาก `POST /api/session` smoke จะแดงจนกว่า ~30 assertion ที่ index `images[...]` จะตามไปด้วย — นั่นคือเหตุผลที่มันเป็น PR แยก
