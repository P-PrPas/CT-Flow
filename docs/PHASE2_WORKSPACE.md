# CT-Flow — Phase 2: Workspace & Multi-user

> **สถานะ: เสร็จครบทั้งสี่ก้อน (T-26–T-35) และ merge เข้า `main` แล้ว** — 2026-08-28
>
> เอกสารนี้เป็นบันทึกของเหตุผลเบื้องหลังการตัดสินใจแต่ละข้อ ไม่ใช่ "สถานะปัจจุบัน" — สถานะอยู่ที่ [ROADMAP.md](./ROADMAP.md) และสิ่งที่เป็นจริงตอนนี้อยู่ที่ [ARCHITECTURE.md](./ARCHITECTURE.md) กับ [API_REFERENCE.md](./API_REFERENCE.md)
> **เอกสารที่เกี่ยวข้อง:** [ARCHITECTURE.md](./ARCHITECTURE.md) · [API_REFERENCE.md](./API_REFERENCE.md) · [REQUIREMENTS.md](./REQUIREMENTS.md) · [ROADMAP.md](./ROADMAP.md)

เอกสารนี้คือแผนงานฉบับเดียวของ Phase 2 — อ่านจบแล้วต้องลงมือเขียนโค้ดได้โดยไม่ต้องถามอะไรเพิ่ม

---

## 1. โจทย์ (สภาพก่อน Phase 2)

ก่อน Phase 2 CT-Flow ไม่มีแนวคิด "โปรเจกต์" ในสายตาผู้ใช้ — โฟลเดอร์ภาพ *คือ* โปรเจกต์ และ path ของมันถูกจำไว้ใน `localStorage` ของ browser คนที่เปิด ผลคือ:

- เปลี่ยนเครื่อง / ล้าง cache แล้วต้องพิมพ์ path ใหม่
- ไม่มีที่ไหนตอบได้ว่า "ในระบบมีงานอะไรอยู่บ้าง ใครทำ ไปถึงไหน"
- สองคนเปิดโฟลเดอร์เดียวกันแล้ว **หยิบภาพเดียวกัน** เพราะคิวเรียงเหมือนกันทุกคน แล้วคนที่กด Save ทีหลังทับคนแรกเงียบ ๆ
- ไม่มีที่ให้ modules อื่น (เช่น license-plate recognition) มาอยู่ในอนาคต — แอปทั้งแอปคือหน้าจอ label detection หน้าเดียว

Phase 2 แก้สี่ข้อนี้ **ไม่แก้อย่างอื่น** — และแก้ครบทั้งสี่แล้ว

## 2. สิ่งที่ตัดสินใจแล้ว (และเหตุผล)

| # | ตัดสินใจ | เหตุผลสั้น ๆ |
|---|---|---|
| 1 | เป้าหมายคือ *รู้ว่าใครทำอะไร* + *แบ่งงานกัน label* ไม่ใช่ระบบสิทธิ์ | ทีมภายใน ไว้ใจกัน ปัญหาที่เจอจริงคือหยิบงานชนกัน ไม่ใช่คนแอบแก้งานคนอื่น |
| 2 | แบ่งงานแบบ **self-serve queue** (จองภาพ + หมดอายุเอง) ไม่ใช่ assignment | คนทำพร้อมกัน 1–2 คนต่อโปรเจกต์ · assignment ต้องมีหน้า assign, re-assign, กติกาตอนคนลาออก — แพงเกินโจทย์ |
| 3 | **`input_dir` ยังเป็น identity ของระบบ** (ไม่เปลี่ยนเป็น `project_id`) | ทุกเมธอดใน `internal/infra/store` และทุก endpoint รับ `input_dir` วันนี้ · เปลี่ยนเป็น `project_id` = แก้ store + 8 handler + `api.ts` + `session.ts` + `smoke_test.py` ทั้งชุด เพื่อความยืดหยุ่นที่ยังไม่มีใครขอ |
| 4 | เผื่ออนาคต **เฉพาะคอลัมน์ `task_type` + โครง routing** ไม่มี abstraction อื่น | abstraction ที่ดีถูก *ค้นพบ* จากโมดูลที่สอง ไม่ได้ถูก *ประดิษฐ์* ก่อนมีโมดูลที่สอง · สิ่งที่ LPR ต้องการเพิ่มทีหลังเป็น `ALTER`/ตารางใหม่ = additive ไม่พังของเดิม |
| 5 | Ownership = คอลัมน์ `owner_oid` · "ใครทำงานในโปรเจกต์นี้" **derive จาก `annotations.created_by`** · ไม่มีตาราง members · ทุกคนที่ login เปิดโปรเจกต์ไหนก็ได้ | ตาราง members เก็บ *ความตั้งใจ* แต่ `created_by` เก็บ *ความจริง* และมีข้อมูลครบอยู่แล้วตั้งแต่ T-21 · ทำให้ดูเหมือนปิดทั้งที่ไม่ได้ปิดจริง แย่กว่าเปิดตรงไปตรงมา |
| 6 | URL: `/` = home, `/p/{project_id}` = workspace | `id` ใน URL แยก "กุญแจสำหรับอ้างถึง" ออกจาก "กุญแจสำหรับเก็บ" — วันที่เปลี่ยนใจไปใช้ `project_id` ทั้งระบบ ลิงก์ที่คนแปะกันไว้ไม่ต้องเปลี่ยนเลย |
| 7 | **บังคับ login** — ไม่มี OIDC และไม่มี `LABEL_TOOL_USERS` = แอปไม่ start · ลบ `LABEL_TOOL_MODE=local` ทิ้ง | ไม่มีใครใช้บนเครื่องตัวเองแล้ว · ตัดสาขา "ไม่มี user" ออกจากทุกหน้าจอ · `PathAllowed` เหลือสาขาเดียว |
| 8 | OIDC สำหรับคนจริง · `LABEL_TOOL_USERS` เหลือไว้เป็นทางเข้าของ CI/dev · `annotations.created_by` ยังเป็น `TEXT` ไม่ใช่ FK | ลบ local login = ต้องเขียน mock OIDC provider ให้ CI เพื่อจะได้ลบโค้ดที่ทำงานดีอยู่แล้ว = ขาดทุนสองต่อ · และ local user ไม่มีแถวใน `users` FK จึงยังใส่ไม่ได้ |
| 9 | ล้าง DB ทิ้ง (เป็น PoC) **และล้าง `.ctflow/` ด้วย** · ไฟล์ภาพห้ามแตะ · ไม่มี migration framework | ล้างครึ่งเดียว = bank จำได้ว่าเคยสอนอะไร แต่ DB จำไม่ได้ว่าสอนจากภาพไหน (ดูข้อ 8 ของเอกสารนี้) |
| 10 | ยังไม่ทำ upload dropzone ในรอบนี้ | ยังไม่มีคำตอบว่าไฟล์ที่อัปโหลดไปลงที่ไหน · เพิ่มทีหลังเป็นตัวเลือกที่สองบนหน้าจอเดิม ไม่ใช่การรื้อ |
| 11 | `smoke_test.py` คือเกณฑ์รับงานของทุกก้อน | มาตรฐานที่ repo นี้ตั้งไว้เองตั้งแต่ Go port — ไม่สร้าง test suite ที่สอง |

**สิ่งที่ Phase 2 ตั้งใจไม่ทำ:** ระบบสิทธิ์/role · ตาราง project members · upload UI · abstraction ต่อ task type · migration framework · deep link ถึงภาพรายตัว · real-time แบบ websocket · ปรับปรุงคุณภาพโมเดล (T-08 ถูกเลื่อน ดู [ROADMAP.md](./ROADMAP.md))

---

## 3. Database schema

### 3.1 สิ่งที่เปลี่ยน

เปลี่ยนที่ `backend/db/schema.sql` ตารางเดียวคือ `projects`:

```sql
CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    input_dir   TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    owner_oid   TEXT,                                       -- users.oid ของคนที่สร้าง
    task_type   TEXT NOT NULL DEFAULT 'detection',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`classes`, `images`, `annotations`, `users` **ไม่เปลี่ยนเลยแม้แต่คอลัมน์เดียว**

### 3.2 ทำไมไม่มี `ALTER TABLE` และไม่มี migration tool

`schema.sql` ถูกรันตอน boot ทุกครั้งและเป็น `CREATE TABLE IF NOT EXISTS` ล้วน ซึ่งแปลว่า **มันอัปเกรดตารางที่มีอยู่แล้วไม่ได้** ทางเลือกมีสามทาง และเราเลือกทางที่สาม:

1. ใส่ `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` คู่กับ `CREATE` — ได้ผล แต่ทำให้ DB ที่สร้างใหม่ได้ `name NOT NULL` ส่วน DB ที่อัปเกรดได้ `name` nullable **สอง schema ที่ไม่เหมือนกันจากไฟล์เดียวกัน** เป็นบ่อเกิดของบั๊กที่หาไม่เจอ
2. เพิ่ม migration framework (goose/Alembic) — dependency ใหม่ + ขั้นตอน deploy ใหม่ เพื่อแก้ปัญหาที่เกิดครั้งเดียว
3. **ล้าง DB ทิ้งแล้วให้ `CREATE TABLE` สร้างใหม่** — ข้อมูลปัจจุบันเป็น PoC ที่เจ้าของยืนยันแล้วว่าทิ้งได้ (ดูข้อ 8)

เพื่อไม่ให้ทางที่ 3 พังแบบเงียบ ๆ ถ้าใครลืมล้าง: **หลังรัน `schema.sql` ตอน boot ให้ตรวจว่าคอลัมน์ที่ Phase 2 ต้องใช้มีอยู่จริง ถ้าไม่มีให้ตายพร้อมข้อความบอกทางแก้** (ดู T-26) — เปลี่ยน `ERROR: column "task_type" does not exist` ที่โผล่ตอน request แรกของผู้ใช้ ให้เป็นข้อความที่อ่านรู้เรื่องตอน start

`ponytail:` เพิ่ม migration framework ตอนที่ schema เริ่มเปลี่ยนบ่อยจริงและล้าง DB ทิ้งไม่ได้แล้ว — ไม่ใช่ตอนนี้

### 3.3 ทำไม "การจองภาพ" ไม่อยู่ใน database

การจองเป็นสถานะชั่วคราวที่หมดอายุใน 10 นาทีและ **ควร**หายไปตอน restart (restart แปลว่าไม่มีใครถืออะไรอยู่แล้ว) เก็บใน PostgreSQL จะได้ผลข้างเคียงที่ไม่ต้องการสองอย่าง:

- แถว `images` วันนี้ถูกสร้างแบบ lazy เฉพาะตอนมีคน label · ถ้าจองต้องเขียน DB ตาราง `images` จะบวมด้วยภาพที่แค่ *เปิดดู* ไม่เคย label
- ต้องมีงานเก็บกวาดแถวที่หมดอายุ

จึงเก็บใน memory ที่ `internal/platform/claims` — โครงเดียวกับ `internal/platform/jobs` ที่มีอยู่แล้ว (map + `sync.Mutex`) และมีข้อจำกัดข้อเดียวกันเป๊ะ: **รองรับ API process เดียว** ซึ่งเป็นข้อจำกัดที่ระบบนี้มีอยู่แล้วและบันทึกไว้แล้วใน [ARCHITECTURE.md](./ARCHITECTURE.md) วันที่ย้าย job tracker ไป Redis (NFR-06) ให้ย้ายอันนี้ไปด้วยกัน

**กติกาการจอง:**
- หนึ่งคนถือได้ **หนึ่งภาพต่อหนึ่งโปรเจกต์** — จองภาพใหม่ = ปล่อยภาพเดิมอัตโนมัติ ไม่ต้องมี endpoint `release`
- TTL 10 นาที นับจากการจองครั้งล่าสุด (`ponytail:` ค่าคงที่ในโค้ด ยังไม่ต้องเป็น env var)
- จองภาพที่คนอื่นถืออยู่ (ยังไม่หมดอายุ) → `409` พร้อมชื่อคนที่ถืออยู่
- การจองเป็น **คำแนะนำ ไม่ใช่ล็อก** — `POST /api/label` ไม่เคยปฏิเสธเพราะเรื่องจอง มันแค่ทำให้คิวของแต่ละคนไม่ชี้ไปที่ภาพเดียวกัน

---

## 4. API ที่เพิ่ม

รูปแบบ error, path safety และ auth ใช้ convention เดิมทุกข้อ (ดู [API_REFERENCE.md](./API_REFERENCE.md))

### 4.1 Projects (`internal/transport/httpapi/projects.go` — ไฟล์ใหม่)

#### `GET /api/projects`
รายการโปรเจกต์ทั้งหมดสำหรับ home page

```json
{"projects": [{
  "id": 12,
  "name": "Cubes conveyor",
  "input_dir": "/opt/mount/project/cubes_conveyor",
  "task_type": "detection",
  "owner": {"oid": "…", "username": "peerapas"},
  "labeled": 34, "auto": 180,
  "contributors": [{"oid": "…", "username": "peerapas", "boxes": 128}],
  "created_at": "…", "updated_at": "…"
}]}
```

- `owner` เป็น `null` ได้ (โปรเจกต์ที่สร้างก่อนมีเจ้าของ) — join `users` เอาชื่อ ไม่ส่ง `oid` ดิบให้ UI แสดง
- `contributors` = `annotations` join `images` join `users` group by `created_by` · คนที่ไม่มีแถวใน `users` (local login ของ CI) แสดง `created_by` ดิบ
- **`labeled`/`auto` มาจาก SQL count ล้วน ไม่มีการอ่านโฟลเดอร์** — จำนวนภาพทั้งหมดในโฟลเดอร์ต้อง `readdir` ซึ่งอาจเป็นหลักพันไฟล์ต่อโปรเจกต์ ไม่คุ้มที่จะทำทุกครั้งที่เปิด home · ตัวเลข "จากทั้งหมดกี่ภาพ" ไปแสดงข้างในโปรเจกต์แทน

#### `POST /api/projects`
- **Body:** `{"name": str, "input_dir": str, "task_type": str}` (`task_type` default `"detection"`)
- **Response:** `{"project": {…}}` (รูปเดียวกับด้านบน)
- `input_dir` ผ่าน `checkedPath()` เหมือนทุก endpoint ที่รับ path · **400** ถ้าโฟลเดอร์ไม่มีอยู่จริงหรือไม่มีภาพเลย (กติกาเดียวกับ `POST /api/session` วันนี้)
- **409** ถ้า `input_dir` นั้นมีโปรเจกต์อยู่แล้ว — ข้อความต้องบอกชื่อโปรเจกต์ที่ครองอยู่ ไม่ใช่แค่ "duplicate"
- **400** ถ้า `task_type` ไม่ใช่ `"detection"` — วันนี้มีค่าเดียว ปฏิเสธค่าอื่นไปเลยดีกว่าปล่อยให้เขียนค่าที่ยังไม่มีใครอ่านลง DB
- `owner_oid` = คนที่เรียก **เสมอ** ไม่รับจาก body

#### `GET /api/projects/{id}`
คืนโปรเจกต์เดียว — หน้า `/p/{id}` เรียกอันนี้ตอน mount เพื่อแปลง `id` เป็น `input_dir` แล้วจากนั้นเรียก endpoint เดิมทั้งหมดด้วย `input_dir` ตามปกติ · **404** ถ้าไม่มี

#### `PATCH /api/projects/{id}`
- **Body:** `{"name"?: str, "claim_ownership"?: true}` — เปลี่ยนชื่อ และ/หรือ รับเป็นเจ้าของ (ตั้ง `owner_oid` เป็นคนที่เรียก)
- `input_dir` และ `task_type` **แก้ไม่ได้** — เปลี่ยนโฟลเดอร์ = โปรเจกต์ใหม่

#### `DELETE /api/projects/{id}`
- ลบแถวใน DB (cascade ไป `classes`/`images`/`annotations`) · **ไม่แตะไฟล์บนดิสก์เลย ทั้งภาพและ `.ctflow/`**
- Response ต้องบอกให้ชัดว่าเหลืออะไรไว้บนดิสก์ เพื่อให้ UI เตือนได้ถูก

### 4.2 Live state (`internal/transport/httpapi/pool.go`)

#### `GET /api/state`
endpoint สำหรับ poll — เบา ไม่แตะ sidecar ไม่แตะดิสก์

- **Query:** `input_dir`
- **Response:** `{"labeled": [path…], "auto": [path…], "testset_labeled": [stem…], "claims": {path: username}}`
- ทุกอย่างมาจาก PostgreSQL + map ใน memory · **จงใจไม่คืน `BankSummary`** เพราะ `classes`/`model` เปลี่ยนเฉพาะตอน *ตัวเอง* label ซึ่ง response ของ `POST /api/label` รีเฟรชให้อยู่แล้ว — ไม่มีเหตุผลให้ยิงไปที่ sidecar ทุก 15 วินาทีต่อ browser ที่เปิดอยู่

#### `POST /api/claim`
- **Body:** `{"input_dir": str, "image": str}`
- **Response:** `{"claims": {path: username}}` (สถานะล่าสุดทั้งโปรเจกต์ ประหยัดการยิงซ้ำ)
- **409** `{"detail": "<username> is working on this image"}` ถ้าคนอื่นถืออยู่และยังไม่หมดอายุ

### 4.3 สิ่งที่เปลี่ยนใน endpoint เดิม

- `POST /api/session` เพิ่ม `"project": {…}` ในคำตอบ **(ทำแล้วใน T-26)** — เป็นจุดสร้างโปรเจกต์อีกจุดผ่าน `EnsureProject` ด้วย ดูหมายเหตุท้าย T-26
- `POST /api/session` เพิ่ม `"bank_orphaned": bool` ในคำตอบ — `true` เมื่อ bank มี embedding แต่ project นี้ไม่มีแถว `images` เลยใน DB (ดูข้อ 8)
- `GET /api/config` **ตัดฟิลด์ `mode` ทิ้ง** พร้อมกับการลบ `LABEL_TOOL_MODE=local` — `roots` ยังอยู่
- `POST /api/upload` ตัดเงื่อนไข `403` เรื่อง "vm mode + ไม่มี auth" ทิ้ง เพราะ auth เป็นสิ่งบังคับแล้ว เงื่อนไขนั้นเป็นจริงเสมอ (endpoint ยังไม่มี UI เรียกในรอบนี้)

---

## 5. Frontend

### 5.1 โครง route

```
/                     home — รายการโปรเจกต์ + สร้างใหม่
/p/{id}               workspace ของโมดูล detection (แท็บ Label/Test set/Report/Progress เดิม)
/entry/login          เดิม
/entry/callback       เดิม
```

`/p/{id}` resolve `id` → `input_dir` ครั้งเดียวตอน mount แล้วส่งเข้า `useSession()` ที่เหลือทำงานเหมือนเดิมทุกประการ · `id` ไม่มีอยู่ → 404 ในแอป ไม่ใช่หน้าขาว

### 5.2 การแบ่งไฟล์ — นี่คือครึ่งที่จับต้องได้ของ "design เผื่อโมดูลอื่น"

```
app/
  page.tsx                        home (ของกลาง — ไม่รู้จักคำว่า YOLOE)
  p/[id]/page.tsx                 workspace shell ของ detection
  components/                     ของกลาง: DirPicker, Confirm, ProgressBar
  lib/
    api.ts                        ของกลาง: request(), auth, config, browse, projects
    ui.tsx                        ของกลาง
  modules/detection/
    session.ts                    ย้ายมาจาก app/lib/
    api.ts                        endpoint เฉพาะ detection: session/label/predict/score/evaluate/autolabel/reembed/testset/history/events/export
    history.ts                    ย้ายมาจาก app/lib/
    panels/{Pool,Testset,Report,Insights}Panel.tsx
    components/{BoxCanvas,ModelPicker,LearningCurve,EvalOverlay,ShortcutsDialog}.tsx
```

**กติกาที่ต้องรักษา:** อะไรก็ตามใต้ `modules/detection/` **ห้ามถูก import จาก `app/page.tsx`** — ถ้าวันหนึ่ง home page ต้องรู้จัก `BankSummary` แปลว่าเส้นแบ่งวางผิดที่ วันที่โมดูล LPR มา มันคือ `modules/lpr/` + route `/p/{id}` ที่แตกแขนงตาม `task_type` ไม่ใช่การแทรกโค้ดเข้าไปในของเดิม

`SetupCard.tsx` ถูกผ่าครึ่ง: ช่องเลือกโฟลเดอร์ + `DirPicker` ย้ายไปอยู่ใน dialog "สร้างโปรเจกต์" บน home · `ModelPicker` อยู่ในโมดูล detection ตามเดิม · dropzone ที่ `disabled` อยู่ให้ย้ายไป home พร้อมกับตัวเลือกโฟลเดอร์ (ยัง `disabled` เหมือนเดิม รอ FR-29)

`localStorage` ที่จำ path (`DIRS_KEY` ใน `session.ts:74`) **ถูกลบทิ้ง** — route คือตัวจำแทน ซึ่งข้ามเครื่องได้

### 5.3 หน้า home

การ์ดหนึ่งใบต่อโปรเจกต์: ชื่อ · ชนิดงาน · เจ้าของ · `labeled`/`auto` · คนที่ลงมือทำ (จาก `contributors`) · แก้ล่าสุดเมื่อไหร่ · ปุ่มเปิด

แบ่งสองส่วน: **ของฉัน** และ **ทั้งหมด** · ไม่มีการซ่อนโปรเจกต์ของใครจากใคร

> ⚠️ **เทียบด้วย `oid` เท่านั้น ห้ามเทียบด้วยชื่อ** — ตอนเขียนแผนข้อนี้เขียนไว้ว่า `owner.oid == me` โดยที่ `me` ไม่เคยมีอยู่จริง: `GET /api/auth/me` คืนแต่ display name (`currentDisplayUser`) ส่วน `owner.oid` คือ `sub` ที่ `currentUser` เขียนลง `owner_oid` — บน OIDC สองค่านี้เป็นคนละสตริงเสมอ ผลคือ "ของฉัน" จะว่างเปล่าทุกกรณีบน deployment จริง และมองไม่เห็นตอน dev เพราะ local account มี attribution เท่ากับ display พอดี · **แก้ใน T-29 โดยเพิ่มฟิลด์ `oid` ใน `GET /api/auth/me`** (และใน response ของ login/logout ให้เหมือนกัน)

ปุ่มสร้าง → dialog: ชื่อ + เลือกโฟลเดอร์ (`DirPicker` เดิม) + ชนิดงาน (วันนี้มีตัวเลือกเดียว แสดงเป็น chip ไม่ใช่ dropdown ที่มีตัวเลือกเดียว)

### 5.4 การรับรู้ว่ามีคนอื่นอยู่

- poll `GET /api/state` ทุก **15 วินาที** ตอนที่หน้า workspace เปิดอยู่ · หยุด poll เมื่อ tab ไม่ active (`document.visibilityState`) — ไม่ต้องยิงให้เครื่องที่ถูกทิ้งไว้ข้ามคืน
- `POST /api/claim` เมื่อเปิดภาพที่ยังไม่มีป้าย · ได้ `409` → แสดงว่าใครถืออยู่ แล้ว **เดินหน้าไปภาพถัดไปให้เอง** ไม่ใช่ค้างอยู่ที่ภาพที่คนอื่นทำ
- `nextTodo` ข้ามภาพที่คนอื่นถืออยู่ · ภาพที่ตัวเองถืออยู่ไม่ข้าม
- คิวแสดงจุดเล็ก ๆ บนภาพที่มีคนถืออยู่ พร้อมชื่อคนใน tooltip
- ในภาพที่ label แล้ว แสดงว่าใครเป็นคน label (จาก `created_by` ที่ `GET /api/boxes` ต้องคืนมาเพิ่ม)

---

## 6. ก้อนงาน

แต่ละก้อน merge เข้า `main` ได้เองโดย `main` ยังทำงานได้ตลอด

### ก้อนที่ 1 — Backend (UI ไม่เปลี่ยน)

#### T-26 · `projects` schema + Projects API ✅
- `backend/db/schema.sql` — เพิ่ม `name`, `owner_oid`, `task_type`, `updated_at` ใน `CREATE TABLE projects` **ตารางอื่นไม่แตะ**
- boot check ใน `Store.InitSchema()` — query `information_schema.columns` ยืนยันว่าคอลัมน์ที่ต้องใช้มีครบ ถ้าไม่ครบ return error ที่บอกทางแก้ตรง ๆ (ล้าง DB ตามหัวข้อ 8)
- `internal/infra/store/projects.go` — `EnsureProject`, `ListProjects`, `GetProject`, `GetProjectByDir`, `UpdateProject`, `DeleteProjectByID`
- `internal/transport/httpapi/projects.go` — 5 endpoint ตามข้อ 4.1
- **เปลี่ยน `getOrCreateProject()` → `requireProject()`** ที่คืน `store.ErrNoProject` ถ้าไม่มีแถว · ทุก write path (`WriteBoxes`, `MarkLabeled`, `MarkAuto`, `MarkTest`, `UnmarkTest`) ปฏิเสธ `input_dir` ที่ยังไม่ถูกสร้างเป็นโปรเจกต์ ด้วย `404 {"detail": "no project for this folder -- create it first"}` แปลงที่ `Handle` ที่เดียว ไม่ใช่ทั้งห้าที่
  - **นี่เป็นการเปลี่ยนพฤติกรรมโดยตั้งใจ** ทำให้ "โปรเจกต์ถูกสร้างอย่างเป็นทางการเท่านั้น" เป็นกติกาจริง ไม่ใช่ธรรมเนียม — ไม่งั้นแถวไร้ชื่อไร้เจ้าของจะงอกขึ้นมาเงียบ ๆ ได้ตลอดเวลา
- **เกณฑ์รับ:** `smoke_test.py` ยืนยัน `409` เมื่อสร้างซ้ำ path เดิม · `404` เมื่อ label ลงโฟลเดอร์ที่ไม่มีโปรเจกต์ · `GET /api/projects` คืน `labeled`/`auto`/`contributors` ตรงกับที่ label ไปจริง · ลบโปรเจกต์แล้วไฟล์ภาพและ prompt bank ยังอยู่ครบ

> **ต่างจากแผนเดิมตอนลงมือทำ (2026-08-28):** แผนบอกว่าก้อนที่ 1 ไม่แตะ UI แต่ถ้า write path ทุกตัวต้องมีโปรเจกต์อยู่ก่อน frontend ปัจจุบันที่เปิด `POST /api/session` แล้ว label เลยจะพังทันทีที่ merge — `main` จะไม่เขียวอย่างที่ตั้งใจ
>
> ทางแก้: **`POST /api/session` เป็นจุดสร้างโปรเจกต์อีกจุดหนึ่ง** ผ่าน `EnsureProject` (ชื่อ = ชื่อโฟลเดอร์, เจ้าของ = คนที่เปิด) ซึ่งซื่อตรงกับความหมายของมันอยู่แล้ว — "เปิดโฟลเดอร์นี้" คือการบอกว่านี่คืองานที่กำลังทำ และมันเป็น call แรกที่ frontend ยิงอยู่แล้ว · กติกา "ทุกแถวมีชื่อและมีเจ้าของ" ยังอยู่ครบ ต่างจาก get-or-create เดิมที่สร้างแถวเปล่าจาก write path ไหนก็ได้ · `POST /api/projects` ยังปฏิเสธ `409` เมื่อสร้างซ้ำ (สร้าง = ตั้งใจ, เปิด = รับช่วง)
>
> ผลข้างเคียงที่ตามมา: response ของ `POST /api/session` มีฟิลด์ `project` เพิ่ม เพื่อให้ client ที่เปิดโฟลเดอร์ตรง ๆ ได้ `id` ไปทำลิงก์ และได้ชื่อ/เจ้าของโดยไม่ต้องยิงซ้ำ

#### T-27 · บังคับ login + ลบ `local` mode ✅
- `cmd/api/main.go` — ตอน boot ถ้าไม่มีทั้ง OIDC config และ `LABEL_TOOL_USERS` → log ข้อความบอกว่าต้องตั้งอะไร แล้ว exit non-zero (แบบเดียวกับที่ compose ทำกับ `POSTGRES_PASSWORD`)
- `internal/platform/config` — ลบ `LABEL_TOOL_MODE` และสาขา `local` ใน `PathAllowed` · `LABEL_TOOL_VM_ROOT` อยู่ต่อและกลายเป็น root เดียวเสมอ
  - **ผลที่ต้องรู้:** การรันนอก Docker ต้องตั้ง `LABEL_TOOL_VM_ROOT` เอง ไม่งั้นทุก path ได้ `403` เพราะ default คือ `/opt/mount/project` (แก้ใน README และใน CI แล้ว)
- `internal/transport/httpapi` — `GET /api/config` ตัดฟิลด์ `mode` · `auth.go` ตัด `authMode() == "none"` และ `state()` ไม่ต้องคืน `enabled: false` อีก · `upload.go` ตัดเงื่อนไข 403 ที่เป็นจริงเสมอแล้ว
- `RequireLogin` ตัด bypass `!authEnabled()` ทิ้ง — **fail closed** ไม่ใช่พึ่ง boot check อย่างเดียว · `authMode()` ไม่มีค่า `"none"` อีกต่อไป
- frontend — `page.tsx` ตัดสาขา `auth.enabled === false` · หน้า login ตัดสาขา `mode === "none"` · `SetupCard` ตัด chip "Shared VM / This machine" · `GET /api/config` ตัดฟิลด์ `mode` ทั้งฝั่ง Go และ TypeScript
- `backend/inference/service.py` — `checked()` ไม่มีเงื่อนไข `MODE == "vm"` อีกแล้ว ฝั่ง sidecar ต้อง confine เท่ากับฝั่ง Go ไม่งั้นขอบเขตแข็งแรงเท่ากับ process ที่ path ไปถึงก่อนเท่านั้น · default เดิมของมันคือ `local` = ไม่ confine เลย
- `.env.example`, `docker-compose*.yml`, `backend/Dockerfile`, `backend/inference/Dockerfile` — ตัด `LABEL_TOOL_MODE`
- **เกณฑ์รับ:** start แอปโดยไม่ตั้ง auth แล้วต้องตายพร้อมข้อความที่อ่านรู้เรื่อง (ไม่ใช่ panic) · `go test ./...` ผ่าน โดย test ของ `config` ที่ครอบ `local` mode ถูกลบ ไม่ใช่ถูก skip

#### T-28 · เปิด auth ใน CI ✅
- `.github/workflows/backend.yml` — ตั้ง `LABEL_TOOL_USERS` (hash จาก `-hash-password`) + `LABEL_TOOL_SECRET` + `SMOKE_USER`/`SMOKE_PASSWORD` ในงาน `smoke`
- **เกณฑ์รับ:** บล็อก auth ใน `smoke_test.py` (ที่ `smoke_test.py:515` เคยข้ามมาตลอด) **เดินจริงบน CI เป็นครั้งแรก** · ยืนยันด้วยการดู log ว่าไม่มีข้อความ "re-run against a server with LABEL_TOOL_USERS set"

### ก้อนที่ 2 — Frontend routing + home

#### T-29 · Home page + route `/p/{id}` ✅
- `app/page.tsx` เขียนใหม่เป็น home · `app/p/[id]/page.tsx` รับช่วง workspace เดิม
- dialog สร้างโปรเจกต์ (ชื่อ + `DirPicker` + chip ชนิดงาน)
- ลบ `DIRS_KEY`/`localStorage` · **ลบ `SetupCard.tsx` ทิ้งทั้งไฟล์** — ช่องเลือกโฟลเดอร์ย้ายไป home, `ModelPicker` มีอยู่ใน `PoolPanel` อยู่แล้ว, และปุ่ม "Open session" ไม่จำเป็นเมื่อ route รู้โฟลเดอร์อยู่แล้ว (`useSession` เปิดเองตอน mount)
- `GET /api/auth/me` (และ login/logout) เพิ่มฟิลด์ `oid` — **แตะ backend ต่างจากที่แผนบอกว่าก้อนนี้เป็น frontend ล้วน** แต่จำเป็น: home page ตอบไม่ได้ว่าโปรเจกต์ไหนเป็นของคนที่ login อยู่ถ้า client ไม่เคยรู้ `oid` ของตัวเอง (ดูกล่องเตือนในข้อ 5.3) · smoke test ยืนยัน shape ใหม่ทั้ง signed-in และ signed-out
- **เกณฑ์รับ:** สร้างโปรเจกต์จาก home → เปิด → label → refresh หน้า → ยังอยู่ที่โปรเจกต์เดิม (route จำให้) · เปิดลิงก์ `/p/{id}` จากอีก browser ที่ login คนละคน แล้วเข้าถึงงานเดียวกันได้ · โปรเจกต์ที่ตัวเองเป็นเจ้าของขึ้นใต้ "Yours" **บน OIDC ไม่ใช่แค่ local account**

#### T-30 · ผ่าโมดูล detection ออกจากของกลาง ✅
- ย้ายไฟล์ตามข้อ 5.2 · แยก `app/lib/api.ts` เป็นของกลาง + `modules/detection/api.ts`
- **เกณฑ์รับ:** `npx tsc --noEmit` + `npm run build` ผ่าน · ไม่มี import จาก `modules/` ในโค้ดส่วนกลาง — **บังคับด้วย CI ไม่ใช่คอมเมนต์** (`.github/workflows/frontend.yml`, ทดสอบแล้วว่า fail จริงเมื่อจงใจละเมิด)
- เพิ่ม workflow `frontend` (boundary check → `tsc --noEmit` → `next build`) — ปิดช่องว่าง "ไม่มี CI ฝั่ง frontend" ที่ความเสี่ยง R4 ระบุไว้ว่าเป็นตาข่ายที่ขาดสำหรับการย้ายไฟล์ทั้งชุดพอดี

### ก้อนที่ 3 — รู้ว่ามีคนอื่นอยู่

#### T-31 · `GET /api/state` + polling ✅
- endpoint ตามข้อ 4.2 · frontend poll ทุก 15 วินาที หยุดเมื่อ tab ไม่ active
- **เกณฑ์รับ:** เปิดสอง browser คนละ user บนโปรเจกต์เดียวกัน · A label ภาพหนึ่ง · ภายใน ~15 วินาที คิวของ B แสดงว่าภาพนั้นเสร็จแล้วโดยที่ B ไม่ต้อง refresh

#### T-32 · การจองภาพ ✅
- `internal/platform/claims` (map + mutex + TTL 10 นาที, โครงเดียวกับ `internal/platform/jobs`) · `POST /api/claim` · `nextTodo` ข้ามภาพที่คนอื่นถือ
- **เกณฑ์รับ:** สอง browser เปิดโปรเจกต์เดียวกันพร้อมกัน แล้ว "ภาพถัดไปที่ควร label" ของสองคนต้องไม่ใช่ภาพเดียวกัน · การจองที่หมดอายุแล้วต้องถูกคนอื่นจองต่อได้ (test ด้วยการหด TTL ผ่านตัวแปรใน test ไม่ใช่รอ 10 นาที)

#### T-33 · แสดงตัวตนของงาน ✅
- `GET /api/boxes` คืน **`labeled_by` ต่อภาพ** (ไม่ใช่ต่อกล่อง — ดูหมายเหตุ) · workspace แสดงชื่อคน label บนภาพที่มีป้ายแล้ว · home แสดง `contributors`

> **ต่างจากแผน:** แผนเขียนว่า "คืน `created_by` ต่อกล่อง" แต่ `Box` เป็น shape ที่ client **ส่งกลับมาด้วย**ทุกครั้งที่ save การแขวน `created_by` ไว้บนนั้นจะเปลี่ยนสัญญาสองทาง ทั้งที่คำถามที่ UI ถามจริงคือ "ใคร label ภาพนี้" → `GET /api/boxes` เพิ่มฟิลด์พี่น้อง `labeled_by: [{oid, username}]` แทน · `Box` ไม่เปลี่ยนเลย
- แปลง `oid` เป็นชื่อผ่าน `users` ที่ฝั่ง Go เสมอ **ไม่ส่ง `oid` ดิบไปให้ UI แสดง**
- **เกณฑ์รับ:** label ด้วยสอง user แล้วเปิดภาพของอีกคน ต้องเห็นชื่อคนที่ label จริง ไม่ใช่ UUID

#### T-34 · ยามตรวจ bank/DB ไม่ตรงกัน ✅
- `POST /api/session` คืน `bank_orphaned: true` เมื่อ bank มี embedding แต่ project ไม่มีแถว `images` เลย · UI ขึ้นแถบเตือนที่อธิบายว่าเกิดอะไรและควรทำอะไร
- **เกณฑ์รับ:** `smoke_test.py` ลบแถว `images` ของ project ทิ้งตรง ๆ แล้วเปิด session ใหม่ ต้องได้ `bank_orphaned: true` (แบบเดียวกับที่มันจำลอง bank เก่าที่ไม่มี key `"model"` อยู่แล้ว)

### ก้อนที่ 4 — เอกสาร

#### T-35 · sync เอกสารกับสิ่งที่ทำจริง ✅
README, ARCHITECTURE, API_REFERENCE, PRODUCT_OVERVIEW, REQUIREMENTS, ROADMAP, GLOSSARY, CLAUDE.md — แก้ให้ตรงกับโค้ดที่ merge ไปแล้ว ไม่ใช่ตรงกับแผนในเอกสารฉบับนี้

สิ่งที่เพิ่มเข้าไปนอกเหนือจากการอัปเดตสถานะ:
- **`GET /api/state`, `POST /api/claim`, `labeled_by`, `bank_orphaned`** เขียนลง API_REFERENCE ครบ (ก่อนหน้านี้ banner บอกว่า "ยังไม่ได้เขียน")
- **โครง frontend ของกลาง/โมดูล** เป็นหัวข้อของตัวเองใน ARCHITECTURE พร้อมกติกาที่ CI บังคับ
- **invariant ใหม่ 3 ข้อใน CLAUDE.md** — subject ไม่ใช่ชื่อ (ข้อ 9), write ต้องมีโปรเจกต์ก่อน (ข้อ 10), ของกลางห้าม import จาก modules (ข้อ 11) · และข้อ 8 ขยายให้ครอบ sidecar ที่ confine แยกต่างหาก
- **ช่องว่าง "ไม่มีการทดสอบที่รัน React"** บันทึกไว้ทั้ง ARCHITECTURE, ROADMAP และ CLAUDE.md พร้อมตัวอย่างจริงที่หลุดผ่านมันมา — ไม่ใช่ช่องว่างเชิงทฤษฎี

---

## 7. ลำดับและสิ่งที่ขวางกัน

```
T-26 ──┬── T-27 ── T-28
       │
       └── T-29 ── T-30 ──┬── T-31 ── T-32
                          └── T-33
                               T-34  (ทำได้ตั้งแต่หลัง T-26)
                                     T-35 ปิดท้าย
```

T-26 ขวางทุกอย่าง · T-31 ขวาง T-32 (จองแล้วไม่มีใครเห็น = ไม่มีประโยชน์) · T-34 ไม่ขวางใครและไม่ถูกใครขวาง แทรกได้ทุกเมื่อ

---

## 8. Reset ก่อน deploy Phase 2 — บังคับ

ข้อมูลปัจจุบันเป็น PoC และ Phase 2 เปลี่ยน schema ของ `projects` โดยไม่มี migration path จึงต้องล้างทั้งสองที่ **พร้อมกัน**

```bash
docker compose down -v                     # ล้าง pgdata

# ล้าง state ของเครื่องมือที่อยู่ข้าง dataset -- ไฟล์ภาพไม่ถูกแตะ
find "$DATA_DIR" -maxdepth 2 -type d -name .ctflow -exec rm -rf {} +

docker compose up --build
```

**ทำไมต้องล้างทั้งคู่:** state ถูกแบ่งกันอยู่คนละที่ตั้งแต่ T-21 — กล่อง/คลาส/สถานะ/test set อยู่ใน PostgreSQL (volume `pgdata`) ส่วน embedding/model lock/eval history อยู่ใน `<dataset>/.ctflow/` (bind mount) `down -v` ล้างแค่ครึ่งแรก ผลคือ:

- `_bank/embeddings.pt` ยังมี embedding ครบ → `Bank.classes` ตอบว่ามีคลาสอยู่
- `_bank/metadata.json` ยังล็อก `model` ไว้ → เปลี่ยนโมเดลไม่ได้ ต้อง reembed
- ตาราง `classes`/`images`/`annotations` ว่าง → UI บอกว่ายังไม่เคย label อะไรเลย
- label ใหม่แล้วบังเอิญเริ่มด้วยคลาสอื่น → `getOrCreateClass` ให้ `idx` ที่ **ไม่ตรงกับลำดับใน `embeddings.pt`** ทั้งที่ [DB_MIGRATION_PLAN.md](./history/DB_MIGRATION_PLAN.md) ข้อ 2.3 ระบุว่าสองอย่างนี้ต้องเป็นลำดับเดียวกัน

วันนี้ยังไม่พังจริงเพราะ prediction ส่งกลับมาเป็นชื่อคลาสไม่ใช่ index — แต่เป็น invariant ที่แตกอยู่โดยไม่มีใครเห็น T-34 ทำให้มันมองเห็นได้

**สำหรับ dev loop ประจำวัน ใช้ `docker compose down` (ไม่มี `-v`)** — rebuild image ครบเหมือนเดิมทุกอย่างแต่ `pgdata` อยู่ต่อ ไม่ต้อง label ใหม่ทุกครั้งที่ทดสอบ · `-v` ใช้เฉพาะตอนตั้งใจล้างจริง **และตอนนั้นต้องลบ `.ctflow/` ด้วยเสมอ**

**สิ่งที่หายไปจริงจากการ reset:** prompt bank (สอนใหม่ได้), `eval_history.json` (กราฟ Progress ย้อนหลัง), `events.jsonl` (usage metric ที่ยังไม่มี UI อ่าน) · ไฟล์ภาพต้นฉบับไม่ถูกแตะแม้แต่ไฟล์เดียว

**ไม่มีสคริปต์ reset ในรีโป** โดยตั้งใจ — สคริปต์ที่ `rm -rf` โฟลเดอร์ dataset ของคนอื่นจะถูกรันผิดครั้งสักวัน

---

## 9. ความเสี่ยงที่รู้ตัว

| # | ความเสี่ยง | ทางรับมือ |
|---|---|---|
| R1 | `getOrCreateProject` → `getProject` ทำให้ flow เดิมที่ยิง `/api/label` ตรง ๆ โดยไม่สร้างโปรเจกต์ก่อน พังทันที | ตั้งใจให้พัง แต่ต้องพังพร้อมข้อความที่บอกทางแก้ · `smoke_test.py` ต้องถูกแก้ให้สร้างโปรเจกต์ก่อนในก้อนเดียวกัน ไม่ใช่ก้อนถัดไป |
| R2 | การจองอยู่ใน memory → API สอง process = จองไม่เห็นกัน | ข้อจำกัดเดียวกับ job tracker ที่มีอยู่แล้ว · บันทึกไว้ใน ARCHITECTURE · ย้ายไป Redis พร้อมกันตอน NFR-06 |
| R3 | polling 15 วินาที × จำนวน browser ที่เปิดค้าง | `GET /api/state` เป็น DB query ล้วน ไม่แตะ sidecar ไม่แตะดิสก์ · หยุด poll เมื่อ tab ไม่ active |
| R4 | ย้ายไฟล์ frontend ทั้งชุด (T-30) แล้ว import พังกระจาย | ทำเป็นก้อนแยกที่ไม่มีการเปลี่ยน logic เลย — ย้ายอย่างเดียว · `npx tsc --noEmit` เป็นเกณฑ์ · ไม่มี frontend CI จึงต้องรันมือก่อน merge (การเพิ่ม frontend type-check เข้า CI อยู่ใน [ROADMAP.md](./ROADMAP.md)) |
| R5 | ลืม reset ตอน deploy → คอลัมน์ไม่ครบ | boot check ใน T-26 ตายพร้อมข้อความบอกทางแก้ ไม่ปล่อยให้พังตอน request แรกของผู้ใช้ |
| R6 | เลื่อน T-08 ออกไป → เครื่องมืออาจยังทำงานไม่ได้ดีพอบน dataset จริง ในขณะที่แพลตฟอร์มรอบมันสวยขึ้น | ตัดสินใจโดยเจ้าของโปรเจกต์แล้ว (2026-08-28) · T-08 ยังอยู่ใน [ROADMAP.md](./ROADMAP.md) เป็นงานถัดไปหลัง Phase 2 · dataset อ้างอิงเปลี่ยนเป็น `cubes_conveyor` ซึ่ง label ง่ายกว่า |
