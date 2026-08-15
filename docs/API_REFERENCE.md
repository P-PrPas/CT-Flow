# Label Tool — API Reference

เอกสารนี้อ้างอิงจากโค้ดจริงใน `backend/routers/*.py` ณ commit ปัจจุบัน ทุก endpoint อยู่ภายใต้ base path `/api` (ยกเว้น testset ที่อยู่ใต้ `/api/testset`) และถูกเรียกผ่าน Next.js proxy (`app/api/[...path]/route.ts`) เสมอ ไม่ใช่ตรงจาก browser

## Convention ที่ใช้ร่วมกัน

- **รูปแบบ error:** ทุก endpoint คืน `HTTPException` ของ FastAPI เมื่อผิดพลาด → body เป็น `{"detail": "<ข้อความ>"}` ฝั่ง frontend (`lib/api.ts`) จับคู่กับสิ่งนี้โดยตรงใน `request()`: โยน `Error(data.detail)` เมื่อ response ไม่ ok
- **Path safety:** endpoint ใดก็ตามที่รับ path จาก browser (`input_dir`, `output_dir`, `test_dir`, รูปภาพ) ต้องผ่าน `deps.checked_path()` ก่อนแตะดิสก์จริง — ถ้า path ไม่ผ่าน `config.path_allowed()` (นอกขอบเขต `vm` mode) จะได้ `403`
- **Box model ที่ใช้ร่วมกันทั้งพูลและ test set:** `{"cls": "<ชื่อคลาส>", "box": [x1, y1, x2, y2]}` พิกัดเป็นพิกเซลจริงของภาพต้นฉบับ (ไม่ normalize)
- **BankSummary** (โครงสร้างที่หลาย endpoint คืนกลับมา): `{"classes": [{"name": str, "count": int}], "labeled": [path...], "auto": [path...]}`
- **Auth:** ถ้าตั้ง `LABEL_TOOL_USERS` ไว้ ทุก endpoint **ยกเว้น** `GET /api/config` และ `/api/auth/*` ต้องมี session cookie ไม่งั้นได้ `401 {"detail": "not signed in"}` · ถ้าไม่ตั้ง ระบบไม่มี login เลยและทุก endpoint เปิดเหมือนเดิม (ดู `services/auth.py`)
- **`conf_by_class`:** `/api/predict`, `/api/evaluate`, `/api/autolabel` รับ dict `{ชื่อคลาส: threshold}` เพื่อ override `conf` เป็นรายคลาส (`{}` = พฤติกรรมเดิม) — เหตุผลและตัวเลขอยู่ใน [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md)

---

## Auth (`routers/auth.py`, prefix `/api/auth`)

ปิดอยู่โดย default ทั้งชุด สร้าง user ด้วย `python -m backend.services.auth <ชื่อ> <รหัสผ่าน>` แล้วใส่ผลลัพธ์ใน `LABEL_TOOL_USERS` (คั่นด้วย comma)

### `GET /api/auth/me`
- **Response:** `{"enabled": bool, "user": str|null}` — `enabled=false` แปลว่าเซิร์ฟเวอร์นี้ไม่มีระบบ login ไม่ใช่ว่ายังไม่ได้ login

### `POST /api/auth/login`
- **Body:** `{"username": str, "password": str}`
- **Response:** `{"enabled": true, "user": str}` + `Set-Cookie: labeltool_session` (HttpOnly, SameSite=Lax, อายุ 12 ชม.)
- **401** เมื่อรหัสผ่านหรือชื่อผู้ใช้ผิด (ข้อความเดียวกันทั้งสองกรณี โดยตั้งใจ)
- **400** ถ้าเซิร์ฟเวอร์ไม่ได้ตั้ง user ไว้เลย

### `POST /api/auth/logout`
- ลบ cookie · **Response:** `{"enabled": bool, "user": null}`

---

## Upload (`routers/uploads.py`)

### `POST /api/upload`
อัปโหลดภาพเข้าโฟลเดอร์ (multipart/form-data)

- **Form:** `dir` (โฟลเดอร์ปลายทาง, สร้างให้ถ้ายังไม่มี), `files` (หลายไฟล์ได้)
- **Response:** `{"saved": [path...], "skipped": [{"name": str, "reason": str}], "images": [path...]}`
- **403** เมื่อ `LABEL_TOOL_MODE=vm` แต่ยังไม่ได้ตั้ง `LABEL_TOOL_USERS` — เงื่อนไขของ T-13: ห้ามเปิดรับไฟล์บนเซิร์ฟเวอร์ที่ใครก็เข้าได้
- **เหตุผลที่ไฟล์ถูกข้าม:** `bad filename` (ชื่อว่าง/ขึ้นต้นด้วย `.`) · `not an image file type` (นามสกุลไม่อยู่ใน `IMAGE_EXTS`) · `not a readable image` (decode ไม่ผ่าน — ด่านจริง ไม่ใช่นามสกุล) · `already in this folder` (ไม่เขียนทับของเดิมเด็ดขาด) · `larger than N MB` (`LABEL_TOOL_MAX_UPLOAD_MB`, default 25)
- ส่วน directory ในชื่อไฟล์ถูกตัดทิ้งเสมอ — `../x.jpg` กลายเป็น `x.jpg` ในโฟลเดอร์ปลายทาง ไม่ใช่ไฟล์นอกโฟลเดอร์

---

## System (`routers/system.py`)

### `GET /api/config`
รายงานโหมดการทำงานปัจจุบัน + root ที่ browse ได้ + สีที่ใช้แสดงกล่องแต่ละคลาส

- **Response:** `{"mode": "local"|"vm", "roots": [str...], "colors": [str...]}`
- ใช้เป็น healthcheck endpoint ของ container `api` ด้วย (`docker-compose.yml`)

### `GET /api/browse`
ข้อมูลสำหรับตัวเลือกโฟลเดอร์ (`DirPicker.tsx`) — แสดง subfolder + จำนวนภาพ

- **Query:** `path` (optional, default `""`)
- **Response:** `{"path": str, "parent": str|null, "dirs": [{"name": str, "path": str}], "images": int, "roots": [str...]}`
- `path=""` คืนแค่รายการ roots
- `404` ถ้า `path` ไม่ใช่ directory
- ข้าม directory ที่ขึ้นต้นด้วย `.` และกลืน `PermissionError` เงียบ ๆ (ไม่ error ทั้ง request)

---

## Pool labeling (`routers/pool.py`)

วงจร label หลักของพูลภาพ

### `POST /api/session`
เปิด session label: ตรวจสอบ input dir, สร้าง/เปิด output dir, list ภาพ, โหลดหรือสร้าง bank

- **Body:** `{"input_dir": str, "output_dir": str}`
- **Response:** `{"images": [str...], "bank": BankSummary}`
- **400** ถ้า input dir ไม่มีอยู่จริงหรือไม่มีภาพเลย

### `GET /api/image`
เสิร์ฟไฟล์ภาพดิบ

- **Query:** `path`
- **Response:** `FileResponse`
- **404** ถ้าไม่ใช่ไฟล์

### `GET /api/boxes`
คืนกล่องที่บันทึกไว้แล้วของภาพหนึ่งภาพ (ใช้ได้ทั้งกับ output_dir ของพูลและ test_dir เพราะ layout เหมือนกัน)

- **Query:** `dir`, `image`
- **Response:** `{"boxes": [Box...]}`

### `POST /api/label`
บันทึกป้าย: สกัด SAVPE embedding เข้า bank ต่อคลาส, mark ภาพว่า labeled, เขียนไฟล์ label YOLO format

- **Body:** `{"output_dir": str, "image": str, "boxes": [Box...], "mode": "replace"|"update"}` (`mode` default `"replace"`)
- **Response:** `{"bank": BankSummary}`
- **400** ถ้าอ่านภาพไม่ได้ หรือ `boxes` ว่างเปล่า
- **พฤติกรรม:** กล่องถูกจัดกลุ่มตามคลาสก่อน แล้วเรียก `extract_embedding()` **หนึ่งครั้งต่อคลาสต่อการบันทึก** (เฉลี่ยจากทุกกล่องของคลาสนั้นในภาพเดียวกัน) → `bank.add()` ต่อคลาส → `bank.mark_labeled()` → `bank.write_yolo_labels()` โดย `merge=True` เมื่อ `mode="update"`
- แต่ละ instance ใน bank บันทึก `labeled_by` = ผู้ใช้ที่ login อยู่ (FR-31) หรือ `null` เมื่อไม่มีระบบ login

### `POST /api/relabel`
เขียนไฟล์ label ของภาพใหม่โดยตรง **ไม่มีการสกัด embedding** — ใช้แก้ป้ายที่ auto-label/review

- **Body:** `{"output_dir": str, "image": str, "boxes": [Box...], "mode": "replace"|"update"}`
- **Response:** `{"bank": BankSummary}`
- **400** ถ้ามี `cls` ใดที่ยังไม่เคยเป็นคลาสใน bank เลย (ข้อความ: `unknown class(es) ... use Save to bank to teach a new class`)
- `boxes` เป็น list ว่างได้ (กรณี "โมเดลทำนายผิดทุกกล่อง" ก็ถือว่าถูกต้อง)

### `POST /api/predict`
กล่องที่โมเดลทำนายไว้สำหรับ **ภาพเดียว** ใช้เป็นกล่องร่างให้ผู้ใช้แก้แทนการวาดใหม่ (FR-19)

- **Body:** `{"output_dir": str, "image": str, "conf": float, "conf_by_class": {cls: float}}`
- **Response:** `{"boxes": [{"cls": str, "box": [...], "conf": float}]}`
- bank ว่าง → `{"boxes": []}` ทันที ไม่มี forward pass
- ไม่แตะ bank และไม่เขียนไฟล์ใด ๆ — กล่องที่คืนมาเป็นข้อเสนอ ยังไม่ใช่ป้าย

### `GET` / `POST` / `DELETE /api/history`
ประวัติผล evaluate เก็บที่ `<output_dir>/_bank/eval_history.json` (T-07) เก็บสูงสุด 200 จุด

- **GET/DELETE query:** `output_dir` · **POST body:** `{"output_dir": str, "point": {...}}`
- **Response:** `{"history": [point...]}` (ทุก method)

### `GET` / `POST /api/events`
สถิติความพยายามของผู้ใช้ (§7) เก็บที่ `<output_dir>/_bank/events.jsonl` แบบ append-only

- **POST body:** `{"output_dir": str, "kind": "session"|"label"|"fix"|"auto", "session": str, "secs": float|null, "written": int}` → `{"ok": true}`
- **GET query:** `output_dir` → `{"summary": {...}}` มี `sessions`, `sessions_reaching_autolabel`, `abandonment`, `manual_labels`, `median_label_secs`, `median_time_to_first_auto_secs`, `auto_written`, `corrections`, `correction_rate`
- ค่าที่ยังไม่มีข้อมูลเป็น `null` ไม่ใช่ `0` — "ยังไม่ได้วัด" กับ "วัดแล้วได้ศูนย์" ต่างกัน

---

## Test set (`routers/testset.py`, prefix `/api/testset`)

Ground truth สำหรับวัดผล ตั้งใจให้แยกขาดจาก prompt bank โดยสิ้นเชิง

### `POST /api/testset/session`
เปิด/สร้างโฟลเดอร์ test set

- **Body:** `{"test_dir": str}`
- **Response:** `{"images": [...], "labeled": [stem...], "classes": [...]}`

### `POST /api/testset/import`
คัดลอกภาพจากพูลเข้าโฟลเดอร์ test set (คัดลอกไฟล์จริง ไม่ย้าย, ข้ามภาพที่ชื่อไฟล์มีอยู่แล้ว)

- **Body:** `{"test_dir": str, "images": [path...]}`
- **Response:** `{"images": [...], "labeled": [...], "classes": [...], "imported": [path ที่คัดลอกจริง]}`
- นำเข้าซ้ำภาพเดิมจะได้ `imported: []` (idempotent)

### `POST /api/testset/remove`
ลบภาพ + ป้าย ground truth ออกจาก test set (ไฟล์ต้นทางในพูลไม่ถูกแตะต้อง; แตะได้แค่ path ที่ resolve อยู่ใน `test_dir` เท่านั้น)

- **Body:** `{"test_dir": str, "images": [path...]}`
- **Response:** `{"images": [...], "labeled": [...], "classes": [...], "removed": [stem...]}`

### `POST /api/testset/label`
เขียนป้าย ground truth ของภาพใน test set

- **Body:** `{"test_dir": str, "image": str, "boxes": [Box...], "mode": "replace"|"update"}`
- **Response:** `{"classes": [...], "labeled": [...]}`
- **400** ถ้าอ่านภาพไม่ได้ หรือ `boxes` ว่างเปล่า

---

## Background jobs (`routers/jobs.py`)

การรัน inference รอบยาวทำงานผ่าน FastAPI `BackgroundTasks` — endpoint ที่สั่งงานคืน `job_id` ทันที ฝั่ง client ต้อง poll เอาเอง

### `GET /api/jobs/{job_id}`
ตรวจสถานะ job

- **Response:** job dict (`{done, total, started_at, finished, result, error}`) รวมกับ `{"now": <server time.time()>}`
- **404** ถ้าไม่รู้จัก `job_id`
- `ProgressBar.tsx` ใช้ `started_at`/`now` จาก server เพื่อคำนวณ ETA แทนการใช้นาฬิกาฝั่ง client (กัน clock skew)

### `POST /api/score`
รีสกอร์ภาพในพูลเทียบกับ bank ปัจจุบัน (background)

- **Body:** `{"output_dir": str, "images": [path...]}`
- **Response:** `{"job_id": str, "total": int}`
- ถ้า bank ว่างเปล่าหรือไม่มี path เลย job จะจบทันทีด้วย `result = {"scores": {}}`
- **ผลลัพธ์ (`result`):** `{"scores": {path: {"conf": float, "cls": str, "sig": [int × 64]}}}` — arm โมเดลครั้งเดียว แล้วรัน `predict_one(conf=0.05)` ต่อภาพ เก็บ detection ที่ confidence สูงสุด · `sig` คือ thumbnail 8×8 เทา ที่ UI ใช้กระจายลำดับภาพไม่ให้เสนอภาพคล้ายกันติดกัน (FR-18)

### `POST /api/evaluate`
ประเมิน bank ปัจจุบันเทียบ test set ที่มีป้ายแล้ว (background)

- **Body:** `{"output_dir": str, "test_dir": str, "conf": float, "conf_by_class": {cls: float}}` (`conf` default `0.25`)
- **Response:** `{"job_id": str, "total": int}`
- **400** ถ้า bank ว่างเปล่า หรือ test set ยังไม่มีป้าย (`metrics.load_ground_truth` โยน `FileNotFoundError`)
- **ผลลัพธ์ (`result`):** `metrics.evaluate(gt, pred)` รวมกับ `{"conf": conf, "conf_by_class": {...}}` — ดูรูปแบบเต็มในหัวข้อ `services/metrics.py` ของ [ARCHITECTURE.md](./ARCHITECTURE.md)

### `POST /api/autolabel`
เขียนป้าย YOLO ให้ภาพที่ระบุโดยตรงจาก bank (background)

- **Body:** `{"output_dir": str, "images": [path...], "conf": float, "conf_by_class": {cls: float}}` (`conf` default `0.25`)
- **Response:** `{"job_id": str, "total": int}`
- **400** ถ้า bank ว่างเปล่า
- **พฤติกรรม:** arm โมเดลครั้งเดียว → predict ทีละภาพ → เขียนไฟล์ label เฉพาะภาพที่มี detection (ไม่งั้นนับเป็น `no_detection`) → `bank.mark_auto()` เฉพาะภาพที่เขียนป้ายจริง
- **ผลลัพธ์ (`result`):** `{"written": int, "no_detection": int, "no_detection_images": [path...], "bank": BankSummary}` — คืนรายชื่อภาพที่ไม่เจออะไรเลย ไม่ใช่แค่จำนวน (FR-28)

**สัญญาร่วมของ background job ทั้งสามตัว:** สร้างผ่าน `job_tracker.create(total)` → tick progress ทีละภาพ → `finish(result)` เมื่อสำเร็จ หรือ `fail(error)` เมื่อ exception — ฝั่ง frontend ใช้ `lib/api.ts`'s `runJob()` POST ไปเริ่ม job แล้ว poll `/api/jobs/{id}` ทุก 400ms จนกว่า `finished`
