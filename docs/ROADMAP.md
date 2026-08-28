# CT-Flow — Roadmap

> **สถานะ ณ 2026-08-28:** backend refactor (Go) และ OIDC login merge เข้า `main` แล้ว · **Phase 2 ก้อนที่ 1 (T-26–T-28) merge แล้ว · ก้อนที่ 2 (T-29, T-30) implement แล้ว** — งานถัดไปคือก้อนที่ 3 แผนเต็มอยู่ที่ [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md)
>
> เอกสารนี้คือ **source of truth ของลำดับงาน** · requirement รายข้ออยู่ที่ [REQUIREMENTS.md](./REQUIREMENTS.md) · บันทึกงานที่จบไปแล้วอยู่ที่ [`history/`](./history/)

## สถานะปัจจุบัน

- API เป็น Go, inference + prompt bank เป็น Python sidecar
- PostgreSQL เก็บ label/box metadata และรองรับหลายคนแก้ project เดียวกันได้ในระดับ storage
- OIDC login ครบทั้ง backend และ frontend · local username/password ยังเป็น fallback
- CI มี `go`, `python`, `smoke` (workflow `backend`) และ `frontend` (boundary → `tsc` → `build`) · `smoke` รันแบบล็อกอินด้วย local account ตั้งแต่ T-28 บล็อก auth จึงเดินจริงทุก push
- โปรเจกต์มีชื่อ/เจ้าของ/ชนิดงาน และมี `/api/projects` ครบห้า endpoint · write path ทุกตัวต้องมีโปรเจกต์อยู่ก่อน · login เป็นสิ่งบังคับ (T-26/T-27)
- ยังเป็นระบบสำหรับทีมภายในบน instance เดียว ไม่ใช่ production-scale deployment

## หลักการจัดลำดับ

แก้สิ่งที่จำเป็นต่อผู้ใช้ก่อน · งาน scale หรือ algorithm ที่ยังไม่มีหลักฐานว่าจำเป็น ให้รอ workload จริงและผลการทดลอง · ไม่เพิ่ม Redis, VRAM eviction, NN-matching หรือ abstraction ต่อ task type เพียงเพราะมีช่องว่างในเอกสาร

---

## Phase 0 — Documentation & deployment hygiene ✅

เสร็จ 2026-08-24 · เอกสารหลัก sync กับ repo · ownership check ของ `DATA_DIR` เป็น deployment preflight ทุกครั้งที่ใช้ path เดิม (ไฟล์ root-owned จากยุคก่อน non-root container เขียนไม่ได้)

## Phase 1 — OIDC Login ✅

เสร็จ 2026-08-24 ตาม flow ของ `corpus-core`

- Authorization-code redirect + server-side code exchange ผ่าน OIDC discovery
- `sub` เป็น audit identity ที่เสถียร · แสดงชื่อจาก `preferred_username` → `email` → `sub`
- CSRF `state` อยู่ใน HttpOnly cookie ตรวจแบบ constant-time ก่อน exchange
- PKCE (S256) และ RP-initiated logout เปิดอัตโนมัติเมื่อ discovery document ประกาศรองรับ
- ตาราง `users` (`oid` = `sub`) upsert ทุก login เพื่อให้ attribution แปลกลับเป็นชื่อคนได้
- provider token อยู่ฝั่ง Go เท่านั้น browser ได้แค่ session cookie อายุ 12 ชั่วโมง

## Phase 2 — Workspace & Multi-user ← **งานปัจจุบัน**

**แผนเต็ม: [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md)** — อ่านเอกสารนั้นก่อนเริ่มเขียนโค้ด

สิ่งที่จะได้: home page ที่ตอบได้ว่าในระบบมีงานอะไร ใครทำ ไปถึงไหน · โปรเจกต์มีชื่อ เจ้าของ ชนิดงาน และ URL ของตัวเอง · สองคน label โปรเจกต์เดียวกันโดยไม่หยิบภาพชนกัน · โครงไฟล์ที่ทำให้โมดูล label แบบอื่นในอนาคตเป็น "โฟลเดอร์พี่น้อง" ไม่ใช่การรื้อของเดิม

| ก้อน | งาน | สรุป |
|---|---|---|
| 1 | T-26 ✅ | `projects` schema (`name`, `owner_oid`, `task_type`) + Projects API + `getOrCreateProject` → `requireProject` |
| 1 | T-27 ✅ | บังคับ login (ไม่มี auth = แอปไม่ start) + ลบ `LABEL_TOOL_MODE=local` |
| 1 | T-28 ✅ | เปิด auth ใน CI ให้บล็อก auth ของ smoke test เดินจริงครั้งแรก |
| 2 | T-29 ✅ | Home page + route `/p/{id}` + ทิ้ง `localStorage` |
| 2 | T-30 ✅ | ผ่าไฟล์โมดูล detection ออกจากของกลาง + workflow `frontend` (boundary → tsc → build) |
| 3 | T-31 | `GET /api/state` + polling ทุก 15 วินาที |
| 3 | T-32 | จองภาพ (in-memory, TTL 10 นาที) กันหยิบงานชนกัน |
| 3 | T-33 | แสดงว่าใคร label กล่องไหน / ใครทำงานในโปรเจกต์นี้ |
| 3 | T-34 | ยามตรวจ prompt bank กับ DB ไม่ตรงกัน |
| 4 | T-35 | sync เอกสารกับสิ่งที่ทำจริง |

**บังคับก่อน deploy:** ล้าง DB **และ** `.ctflow/` พร้อมกัน — ดูหัวข้อ 8 ของ [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md)

**สิ่งที่ Phase 2 ตั้งใจไม่ทำ:** role/permission · ตาราง project members · upload UI · abstraction ต่อ task type · migration framework · websocket

## Phase 3 — Model quality: small-object classes

**เลื่อนมาจากตำแหน่งเดิม (เคยเป็น Phase 2) ตามการตัดสินใจของเจ้าของโปรเจกต์ 2026-08-28** — workspace มาก่อนเพราะเป็นงานที่รู้ว่าจบตรงไหน ส่วน T-08 เป็นการทดลองที่ผลอาจเป็น "ไม่ช่วย"

- **T-08 · crop-before-SAVPE** — แก้ปัญหาคลาส `defect` ที่ยังได้ F1 ~0.25 แม้ใช้ per-class threshold แล้ว
- วัด precision/recall/F1 เทียบ baseline บน dataset อ้างอิง (**เปลี่ยนจาก `iron_ore` เป็น `cubes_conveyor`** เพราะ label ง่ายกว่า) และเทียบกับ `conveyor_pvc` ที่มีตัวเลขเดิมอยู่แล้ว
- **T-11 · NN-matching bank** — ทำต่อ *เฉพาะเมื่อ* ผลการทดลองชี้ว่าปัญหาคือ class variation จริง ไม่ใช่ resolution

**ห้ามเริ่มด้วยการเปลี่ยน bank algorithm โดยไม่มีผลทดลองรองรับ** — mean-pooling ยังเพียงพอสำหรับคลาสที่ลักษณะสม่ำเสมอ (เหตุผลเต็มที่ [history/EXPERIMENT_T01_CONF.md](./history/EXPERIMENT_T01_CONF.md))

## Phase 4 — UX และ product completeness

ทำเมื่อมีผู้ใช้จริงยืนยันความต้องการ:

- **T-13 · Upload dropzone** ที่เรียก `POST /api/upload` (backend เสร็จแล้ว) — ต้องตอบก่อนว่าไฟล์ที่อัปโหลดไปลงโฟลเดอร์ไหน ใครตั้งชื่อ ลบโปรเจกต์แล้วไฟล์หายไหม
- ส่ง usage event จาก frontend ให้สถิติข้าม session และข้ามเครื่องได้จริง (backend สรุปได้แล้ว ดู REQUIREMENTS §7)
- เพิ่ม frontend type-check/build เข้า CI — **ควรทำก่อน T-30** ถ้ามีเวลา เพราะการย้ายไฟล์ทั้งชุดไม่มีตาข่ายรองรับเลยตอนนี้
- export-format picker บน UI (`GET /api/export` รองรับ YOLO/COCO/VOC อยู่แล้ว)

## Phase 5 — Scale และ operations (conditional)

ทำเฉพาะเมื่อมี requirement รองรับ traffic หรือหลาย instance:

- **T-15 · ย้าย job tracker ไป Redis + TTL** — และย้าย image claims ไปพร้อมกัน ทั้งสองมีข้อจำกัด "process เดียว" ข้อเดียวกัน
- VRAM eviction/LRU เมื่อการสลับหลาย checkpoint ทำให้เกิด OOM จริง
- HTTPS/reverse proxy, deployment pipeline, monitoring

## Research backlog

ยังไม่ใช่งานถัดไปโดยอัตโนมัติ — เริ่มเมื่อมี baseline จากการใช้งานจริงและมีคำถามเชิงผลิตภัณฑ์ที่ชัดเจน:

- active-learning selector ที่ใช้ uncertainty/diversity จริง (วันนี้ใช้ลายนิ้วมือภาพ 8×8 แทนระยะห่าง embedding)
- retrain closed-set detector จาก annotation ที่สะสม
- โมดูล label แบบอื่น (เช่น license-plate recognition) — โครงรองรับหลังจบ Phase 2 แต่ **ยังไม่มีการออกแบบ** และไม่ควรเริ่มจนกว่าจะมีงานจริงรออยู่

---

## Decision gates

- **Workspace:** ทำแล้ว (Phase 2) · เพิ่ม role/permission เฉพาะเมื่อมี policy จริง ไม่ใช่เพราะ "น่าจะมี"
- **Model quality:** ทำ crop experiment ก่อน NN-matching เสมอ
- **Scale:** วัดจำนวนผู้ใช้, instance และ job concurrency ก่อนเพิ่ม Redis/eviction
- **โมดูลที่สอง:** เริ่มเมื่อมีงานจริง ไม่ใช่เพื่อพิสูจน์ว่าโครงรองรับ — abstraction ถูกค้นพบจากโมดูลที่สอง ไม่ได้ถูกประดิษฐ์ก่อน
- **Production:** กำหนด security, deployment และ observability requirements ก่อนเปิดใช้งานนอกทีมภายใน

## ข้อจำกัดที่รู้ตัวและตั้งใจปล่อยไว้

| ข้อจำกัด | เงื่อนไขที่จะทำให้ต้องแก้ |
|---|---|
| Job tracker + image claims อยู่ใน memory ของ process เดียว ไม่ persist ไม่มี TTL cleanup | ต้องรัน API มากกว่าหนึ่ง process |
| โมเดลถูก cache ต่อ `model_id` ต่อ process ไม่มี VRAM eviction | สลับหลาย checkpoint ใหญ่ในโปรเซสเดียวจน OOM จริง |
| ตรวจภาพซ้ำด้วย thumbnail hash 8×8 ไม่ใช่ระยะห่าง embedding จริง | มีหลักฐานว่ามันเลือกภาพผิดจนเสียเวลา label (การเปลี่ยนจะทำให้ rescore ช้าขึ้นราวเท่าตัว) |
| ไม่มี HTTPS ในตัวแอป | deploy นอก network ที่ควบคุมได้ — แก้ด้วย reverse proxy ไม่ใช่แก้ในแอป |
| ไม่มี integration test ฝั่ง frontend | มี regression ที่ `tsc` จับไม่ได้เกิดซ้ำ |
| `annotations.created_by` เป็น `TEXT` ไม่ใช่ FK ไป `users(oid)` | เลิกใช้ `LABEL_TOOL_USERS` (local login ไม่มีแถวใน `users`) |
