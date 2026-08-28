# CT-Flow — API Reference

> **Phase 2 ก้อนที่ 1 merge แล้ว** — `/api/projects*` อยู่ในเอกสารนี้แล้ว · `GET /api/state` กับ `POST /api/claim` ยังเป็นแผน สัญญาของสองตัวนั้นอยู่ที่ [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) ข้อ 4 และจะย้ายมาที่นี่เมื่อ merge เอกสารนี้อธิบายสิ่งที่มีอยู่จริงตอนนี้เท่านั้น

เอกสารนี้อ้างอิงจากโค้ดจริงใน `internal/transport/httpapi/*.go` ณ commit ปัจจุบัน ทุก endpoint อยู่ภายใต้ base path `/api` (ยกเว้น testset ที่อยู่ใต้ `/api/testset`) และถูกเรียกผ่าน Next.js proxy (`app/api/[...path]/route.ts`) เสมอ ไม่ใช่ตรงจาก browser

> **หมายเหตุหลัง port เป็น Go:** เอกสารนี้คือ reference ฉบับเดียว — ไม่มี Swagger UI / `/openapi.json` อีกแล้ว (FastAPI แถมมาให้ ส่วน Go ไม่มี และ spec ที่ generate จาก `req: dict` ได้แค่ `body: object` ซึ่งด้อยกว่าเอกสารนี้อยู่แล้ว) · **request/response ทุกตัวในเอกสารนี้ไม่เปลี่ยนเลยจากตอนเป็น FastAPI** — ยืนยันด้วย `backend/tests/parity.py` ที่ diff response ทีละฟิลด์ระหว่างสอง backend ได้ 43/43 เหมือนกันหมด

## Convention ที่ใช้ร่วมกัน

- **รูปแบบ error:** ทุก endpoint คืน body เป็น `{"detail": "<ข้อความ>"}` เมื่อผิดพลาด ฝั่ง frontend (`lib/api.ts`) จับคู่กับสิ่งนี้โดยตรงใน `request()`: โยน `Error(data.detail)` เมื่อ response ไม่ ok · ฝั่ง Go บังคับด้วยการให้ทุก handler คืน `error` แทนการเขียน response เอง (`internal/transport/httpapi.Handle`) จึงไม่มี handler ไหนลืมรูปแบบนี้ได้ · **ข้อความ error ทุกตัวยกมาเหมือนเดิมทุกตัวอักษร** เพราะ smoke test เทียบตรง ๆ และ UI เอาไปแสดง
- **Path safety:** endpoint ใดก็ตามที่รับ path จาก browser (`input_dir`, รูปภาพ) ต้องผ่าน `Server.checkedPath()` ก่อนแตะดิสก์จริง — ถ้า path ไม่ผ่าน `config.PathAllowed()` (resolve ออกนอก `LABEL_TOOL_VM_ROOT`) จะได้ `403` — การจำกัดขอบเขตเป็น **unconditional** ตั้งแต่ T-27 ไม่มีโหมดที่ปิดมันได้อีกแล้ว · การตรวจใช้การ resolve symlink แล้วเทียบเป็น path component ไม่ใช่ prefix ของ string (มี test ครอบ 6 เคสใน `internal/platform/config`)
- **`input_dir`:** ทุก endpoint ที่ทำงานกับ project ใดโปรเจกต์หนึ่งรับแค่ `input_dir` ตัวเดียว — prompt bank อยู่ใต้ subfolder ตายตัว `<input_dir>/.ctflow/` (ดู `deps.state_dir()`) ส่วนป้ายและ test-set membership อยู่ใน PostgreSQL คีย์ด้วย `input_dir` เดียวกัน (T-21, ดู `internal/infra/store`) ไม่มี output folder หรือ test-set folder ให้เลือกแยกอีกต่อไป
- **Box model ที่ใช้ร่วมกันทั้งพูลและ test set:** `{"cls": "<ชื่อคลาส>", "box": [x1, y1, x2, y2]}` พิกัดเป็นพิกเซลจริงของภาพต้นฉบับ (ไม่ normalize)
- **BankSummary** (โครงสร้างที่หลาย endpoint คืนกลับมา): `{"classes": [{"name": str, "count": int}], "labeled": [path...], "auto": [path...], "model": str|null}` — `model` เป็น `null` จนกว่าจะมี embedding แรกเข้า bank แล้วล็อกตลอดไป (ดู `POST /api/label`)
- **Auth (บังคับตั้งแต่ T-27):** ทุก endpoint **ยกเว้น** config/login routes ต้องมี `labeltool_session` cookie ไม่งั้นได้ `401 {"detail": "not signed in"}` · ต้องตั้ง OIDC (`OAUTH_CLIENT_ID`, `OAUTH_CLIENT_SECRET`, `OAUTH_ENDPOINT`, `FRONTEND_URL`) หรือ `LABEL_TOOL_USERS` อย่างใดอย่างหนึ่ง **ไม่ตั้งเลย = API ไม่ start** · OIDC มี priority เหนือ local fallback
- **`404 {"detail": "no project for this folder -- create it first"}`:** ทุก write path (`/api/label`, `/api/relabel`, `/api/testset/import`, `/api/testset/remove`, `/api/testset/label`, และ job ที่เขียนสถานะอย่าง `/api/autolabel`) ปฏิเสธ `input_dir` ที่ยังไม่มีแถวใน `projects` — ก่อน Phase 2 write path สร้างแถวให้เองเงียบ ๆ ทำให้เกิดโปรเจกต์ไร้ชื่อไร้เจ้าของจาก path ที่พิมพ์ผิด · สร้างโปรเจกต์ได้สองทางเท่านั้น: `POST /api/projects` หรือ `POST /api/session` (ดูหัวข้อ Projects) · แปลงจาก `store.ErrNoProject` ที่ `Handle` ที่เดียว status กับข้อความจึงไม่มีทางเพี้ยนกันระหว่าง endpoint
- **`conf_by_class`:** `/api/predict`, `/api/evaluate`, `/api/autolabel` รับ dict `{ชื่อคลาส: threshold}` เพื่อ override `conf` เป็นรายคลาส (`{}` = พฤติกรรมเดิม) — เหตุผลและตัวเลขอยู่ใน [EXPERIMENT_T01_CONF.md](./history/EXPERIMENT_T01_CONF.md)

---

## Auth (`internal/transport/httpapi/auth.go`)

ปิดอยู่โดย default ทั้งชุด OIDC ใช้ authorization-code flow แบบเดียวกับ `corpus-core`; backend exchange code และออก application-session cookie โดยไม่ส่ง provider token กลับ browser

### `GET /api/public/login/redirect`
- **Response:** `{"redirectUrl": str}` + HttpOnly state cookie อายุ 5 นาที
- state cookie เก็บ `<state>.<PKCE verifier>` ไว้ด้วยกัน (ทั้งสองส่วนเป็น base64url จึงไม่มี `.` ปนแน่นอน) — cookie เดียวที่มาไม่ครบไม่ได้ ดีกว่าสอง cookie ที่มาครึ่งเดียวได้
- แนบ `code_challenge` (S256) **เฉพาะเมื่อ discovery document ของ provider ประกาศ `code_challenge_methods_supported: ["S256"]`** — provider ที่ไม่รองรับจะไม่ได้รับ parameter ที่ไม่ได้ขอ

### `POST /api/public/login/callback`
- **Body:** `{"code": str, "state": str}`
- **Response:** `{"enabled": true, "user": str, "oid": str, "mode": "oidc"}` + `Set-Cookie: labeltool_session` (HttpOnly, SameSite=Lax, อายุ 12 ชม.) — `oid` คือ `sub` ของ provider ส่วน `user` คือ display name
- **401** เมื่อ state ไม่ตรง, code exchange ล้มเหลว หรือ user-info ใช้ไม่ได้ — เทียบ state แบบ constant-time และลบ state cookie **ก่อน** แลก code
- สำเร็จแล้ว upsert แถวใน `users` (`oid` = `sub` ของ provider) เพื่อให้ `sub` ที่ไปอยู่ใน `annotations.created_by` / `labeled_by` แปลกลับเป็นชื่อคนได้ · เขียนไม่สำเร็จ **ไม่** ทำให้ login พัง (เป็นปัญหาฝั่ง reporting ไม่ใช่เหตุผลที่จะปฏิเสธ login ที่ถูกต้อง)

### `GET /api/auth/me`
- **Response:** `{"enabled": true, "user": str|null, "oid": str|null, "mode": "local"|"oidc"}` — `user: null` แปลว่ายังไม่ได้ login · `enabled` เป็น `true` เสมอตั้งแต่ T-27 (ไม่มีเซิร์ฟเวอร์ที่ไม่มี login อีกแล้ว) เก็บฟิลด์ไว้เพราะ frontend กับ smoke test อ่านมันอยู่ การลบฟิลด์เป็น breaking change คนละเรื่องกับ T-27
- **`oid` คือกุญแจ attribution ของคนที่เรียก** — ค่าเดียวกับที่ลงใน `projects.owner_oid` และ `annotations.created_by` · `user` คือ**ชื่อที่เอาไว้แสดง** ไม่ใช่ identity: บน OIDC มันคือ display name ที่ provider เปลี่ยนได้และซ้ำกันได้ ส่วน `oid` คือ `sub` · UI ต้องเทียบด้วย `oid` เมื่อจะตอบว่า "อันนี้ของฉันหรือเปล่า" (T-29) · บน local account ทั้งสองค่าเท่ากันคือ username ซึ่งเป็นเหตุผลที่การเทียบผิดจะดูถูกต้องตอน dev · `user` กับ `oid` มาคู่กันเสมอ ไม่มีทางที่ตัวหนึ่ง null อีกตัวไม่ null

### `POST /api/auth/login`
- **Body:** `{"username": str, "password": str}`
- **Response:** `{"enabled": true, "user": str, "oid": str, "mode": "local"}` + `Set-Cookie: labeltool_session` (HttpOnly, SameSite=Lax, อายุ 12 ชม.) — local account ไม่มี subject แยก `oid` จึงเท่ากับ username
- **401** เมื่อรหัสผ่านหรือชื่อผู้ใช้ผิด (ข้อความเดียวกันทั้งสองกรณี โดยตั้งใจ)
- **400** ถ้า OIDC active อยู่ (local login เป็น fallback เท่านั้น) — เคส "เซิร์ฟเวอร์ไม่มี user เลย" ไม่มีอีกแล้ว process ตายตั้งแต่ boot

### `POST /api/auth/logout`
- ลบ cookie · **Response:** `{"enabled": true, "user": null, "oid": null, "mode": "local"|"oidc", "logoutUrl"?: str}`
- `logoutUrl` มีเฉพาะโหมด `oidc` และเฉพาะเมื่อ discovery document มี `end_session_endpoint` — frontend ต้องพา browser ไปที่ URL นั้น ไม่งั้น session ฝั่ง provider ยังอยู่ แล้วการกด "sign in" ครั้งถัดไปบนเครื่อง label ที่ใช้ร่วมกันจะ login เงียบ ๆ เป็นคนเดิม
- **ไม่**แนบ `post_logout_redirect_uri`: parameter นั้นต้อง register กับ provider ก่อน และ logout ที่พังเพราะ URL ไม่ได้ register แย่กว่า logout ที่จบบนหน้า signed-out ของ provider เอง

---

## Upload (`internal/transport/httpapi/upload.go`)

### `POST /api/upload`
อัปโหลดภาพเข้าโฟลเดอร์ (multipart/form-data)

- **Form:** `dir` (โฟลเดอร์ปลายทาง, สร้างให้ถ้ายังไม่มี), `files` (หลายไฟล์ได้)
- **Response:** `{"saved": [path...], "skipped": [{"name": str, "reason": str}], "images": [path...]}`
- เงื่อนไขของ T-13 ("ห้ามเปิดรับไฟล์บนเซิร์ฟเวอร์ที่ใครก็เข้าได้") เคยเป็น `403` ตรงนี้ — **ตัดออกแล้วใน T-27** เพราะเป็นจริงโดยโครงสร้าง: process ไม่ start ถ้าไม่มี login เงื่อนไขนั้นจึงเกิดไม่ได้
- **เหตุผลที่ไฟล์ถูกข้าม:** `bad filename` (ชื่อว่าง/ขึ้นต้นด้วย `.`) · `not an image file type` (นามสกุลไม่อยู่ใน `IMAGE_EXTS`) · `not a readable image` (decode ไม่ผ่าน — ด่านจริง ไม่ใช่นามสกุล) · `already in this folder` (ไม่เขียนทับของเดิมเด็ดขาด) · `larger than N MB` (`LABEL_TOOL_MAX_UPLOAD_MB`, default 25)
- ส่วน directory ในชื่อไฟล์ถูกตัดทิ้งเสมอ — `../x.jpg` กลายเป็น `x.jpg` ในโฟลเดอร์ปลายทาง ไม่ใช่ไฟล์นอกโฟลเดอร์

---

## System (`internal/transport/httpapi/system.go`)

### `GET /api/config`
รายงาน root ที่ browse ได้ + สีที่ใช้แสดงกล่องแต่ละคลาส + รายการโมเดลที่เลือกได้

- **Response:** `{"roots": [str...], "colors": [str...], "models": [ModelInfo...], "default_model": str}`
- **ไม่มีฟิลด์ `mode` แล้ว** (T-27) — มันรายงาน `local` vs `vm` ซึ่งตอนนี้เหลือพฤติกรรมเดียว `roots` ยังอยู่
- `ModelInfo` = `{"id": str, "family": str, "size": str, "note": str, "available": bool}` — ดู [`backend/models.json`](../backend/inference/models.py), `id` คือค่าที่ส่งเป็น `model_id` ใน `POST /api/label` · `available` เช็คสดจากดิสก์ทุกครั้งที่เรียก (มีไฟล์ `.pt` อยู่ใน `MODELS_DIR` จริงหรือไม่) — `false` ไม่ได้แปลว่าใช้ไม่ได้ แค่แปลว่า predict/label ครั้งแรกด้วยโมเดลนั้นจะไป auto-download จาก GitHub ก่อน (อาจช้าหรือเงียบล้มเหลวถ้าเน็ตไปไม่ถึง)
- ใช้เป็น healthcheck endpoint ของ container `api` ด้วย (`docker-compose.yml`)

### `GET /api/browse`
ข้อมูลสำหรับตัวเลือกโฟลเดอร์ (`DirPicker.tsx`) — แสดง subfolder + จำนวนภาพ

- **Query:** `path` (optional, default `""`)
- **Response:** `{"path": str, "parent": str|null, "dirs": [{"name": str, "path": str}], "images": int, "roots": [str...]}`
- `path=""` คืนแค่รายการ roots
- `404` ถ้า `path` ไม่ใช่ directory
- ข้าม directory ที่ขึ้นต้นด้วย `.` และกลืน `PermissionError` เงียบ ๆ (ไม่ error ทั้ง request)

---

## Projects (`internal/transport/httpapi/projects.go`, T-26)

โปรเจกต์คือโฟลเดอร์ dataset หนึ่งโฟลเดอร์ที่มีคนลงทะเบียนไว้ พร้อมชื่อ เจ้าของ และชนิดงาน

**`id` ใช้สำหรับอ้างถึง `input_dir` ใช้สำหรับเก็บ และสองอย่างนี้ไม่สลับหน้าที่กัน** — endpoint อื่นทุกตัวยังรับ `input_dir` เหมือนเดิมทั้งหมด `id` มีไว้ให้ UI เอาไปใส่ URL (`/p/{id}`) โดยไม่ต้องเอา path ของเซิร์ฟเวอร์ไปแปะ (ดู [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) ข้อ 2 decision 6)

**Project** = `{"id": int, "input_dir": str, "name": str, "task_type": "detection", "owner": Person|null, "labeled": int, "auto": int, "contributors": [Contributor...], "created_at": str, "updated_at": str}`

- `Person` = `{"oid": str, "username": str}` — `username` fallback เป็น `oid` เมื่อไม่มีแถวใน `users` (คือ local login ทั้งหมด รวม CI)
- `Contributor` = `{"oid": str, "username": str, "boxes": int}` — **derive จาก `annotations.created_by`** ไม่ใช่ตาราง members: เก็บว่าใคร label จริง ไม่ใช่ใครถูกเชิญ (FR-50) เรียงตามจำนวนกล่องมากไปน้อย
- `labeled`/`auto` นับจาก `images.status` ใน SQL (เฉพาะ `kind = 'pool'`) **ไม่มีการอ่านโฟลเดอร์** — card ที่เขียนว่า "34 of 3,000" ไม่ควรแลกมาด้วย readdir ต่อโปรเจกต์ทุกครั้งที่โหลดหน้า home

### `GET /api/projects`
- **Response:** `{"projects": [Project...]}` เรียงตาม `updated_at` ใหม่สุดก่อน
- **ไม่กรองตามเจ้าของ** — UI แบ่ง "ของฉัน"/"ทั้งหมด" เอง การซ่อนงานคนอื่นที่ระดับ endpoint จะเป็นการสร้าง permission boundary ซึ่ง Phase 2 ตั้งใจไม่มี (ใครที่ login แล้วก็เดินไปโฟลเดอร์ไหนก็ได้ผ่าน `GET /api/browse` อยู่แล้ว)
- สองคำสั่ง SQL เสมอไม่ว่าจะมีกี่โปรเจกต์: หนึ่งคำสั่งสำหรับแถว+ตัวนับ อีกหนึ่งสำหรับ contributor ทั้งหมด

### `POST /api/projects`
- **Body:** `{"name": str, "input_dir": str, "task_type"?: "detection"}`
- **Response:** `{"project": Project}` — เจ้าของคือคนที่เรียก
- **400** `a project needs a name` (ชื่อว่างหรือมีแต่ช่องว่าง) · `unknown task type: <x>` (ค่าอื่นนอกจาก `detection` — เก็บค่าที่ยังไม่มีโมดูลไหนรับผิดชอบไว้ = สัญญาที่แอปทำไม่ได้) · `input dir not found: <dir>` · `no images in <dir>` (สองอันหลังคือด่านเดียวกับที่ `POST /api/session` ตรวจมาตลอด แค่ย้ายมาตรวจตอนเลือกโฟลเดอร์แทนตอนจะ label)
- **403** ถ้า `input_dir` อยู่นอก `LABEL_TOOL_VM_ROOT`
- **409** `this folder is already the project "<ชื่อ>"` — ไม่รับช่วงเงียบ ๆ เด็ดขาด คนที่สั่ง "สร้าง" ไม่รู้ตัวว่ากำลังจะเข้าไปร่วมงานของคนอื่น และชื่อคือสิ่งที่บอกเขาว่าของใคร · **สร้าง = ตั้งใจ, เปิด = รับช่วง** (`POST /api/session` รับช่วงให้)

### `GET /api/projects/{id}`
- **Response:** `{"project": Project}` — `/p/{id}` เรียกตอน mount เพื่อแปลง id ใน URL เป็น `input_dir` ที่ endpoint อื่นพูดกัน
- **404** `no such project` — รวมถึงกรณี id ไม่ใช่ตัวเลข

### `PATCH /api/projects/{id}`
- **Body:** `{"name"?: str, "claim_ownership"?: bool}` — ส่งมาอย่างใดอย่างหนึ่งหรือทั้งคู่ก็ได้
- **Response:** `{"project": Project}`
- **400** `a project needs a name` (ส่ง `name` มาแต่ว่าง) · `nothing to update` (ไม่ได้ส่งอะไรมาเลย)
- **404** `no such project`
- **`claim_ownership` เติมเจ้าของที่ว่างอยู่เท่านั้น ไม่แย่งของใคร** — โปรเจกต์ที่มีเจ้าของแล้วเรียกไปก็ไม่เปลี่ยน (SQL เป็น `COALESCE(owner_oid, $3)`) และไม่มีฟิลด์สำหรับระบุชื่อคนอื่น เพราะ "โอนให้ Bob" เป็นคำถามเรื่องสิทธิ์ที่ Phase 2 ตั้งใจไม่ตอบ
- เปลี่ยน `input_dir` หรือ `task_type` ไม่ได้: ชี้ไปโฟลเดอร์อื่น = คนละโปรเจกต์ ส่วนเปลี่ยนชนิดงานจะทำให้ label ที่มีอยู่กำพร้า
- rename ไม่ล้างเจ้าของ และ claim ไม่ rename — ฟิลด์ที่ไม่ได้ส่งมาคงเดิมเสมอ

### `DELETE /api/projects/{id}`
- **Response:** `{"deleted": int, "kept_on_disk": str}`
- **404** `no such project` (เรียกซ้ำได้ ครั้งที่สองได้ 404)
- ลบ `classes`, `images`, `annotations` ตาม cascade — **ไม่แตะไฟล์บนดิสก์เลย** ทั้งไฟล์ภาพและ prompt bank ใน `.ctflow/` `kept_on_disk` บอกตรง ๆ ว่าอะไรยังอยู่ เพื่อให้ UI บอกผู้ใช้ได้
- ⚠️ **เพดานที่รู้ตัว:** ลบแล้วเปิดโฟลเดอร์เดิมอีกครั้งจะได้โปรเจกต์ใหม่ที่ `classes.idx` เริ่มที่ 0 ขณะที่ `_bank/embeddings.pt` ยังจำลำดับคลาสเก่าและ `metadata.json` ยังล็อก checkpoint เก่า — เป็น DB/bank split ตัวเดียวกับ [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) ข้อ 8 วันนี้ยังไม่กัดเพราะ prediction คืนมาเป็นชื่อคลาสไม่ใช่ index · ล้าง bank ตรงนี้ไม่ใช่ทางแก้ (invariant 5: มีแต่ sidecar ที่แตะไฟล์สองตัวนั้นได้) ทางแก้ที่วางไว้คือ `bank_orphaned` ใน `POST /api/session` (ข้อ 4.3)

---

## Pool labeling (`internal/transport/httpapi/pool.go`, `label.go`, `project.go`)

วงจร label หลักของพูลภาพ

### `POST /api/session`
เปิด session label: ตรวจสอบ input dir, list ภาพ, โหลดหรือสร้าง bank ใต้ `<input_dir>/.ctflow/` — คืน state ของ test set มาในคำตอบเดียวกันเลย ไม่ต้องเรียกแยก

- **Body:** `{"input_dir": str}`
- **Response:** `{"images": [str...], "bank": BankSummary, "testset": {"images": [str...], "labeled": [stem...], "classes": [str...]}, "project": Project}`
- **400** ถ้า input dir ไม่มีอยู่จริงหรือไม่มีภาพเลย
- **สร้างโปรเจกต์ให้ถ้าโฟลเดอร์นี้ยังไม่มี** (`EnsureProject` — ชื่อ = ชื่อโฟลเดอร์, เจ้าของ = คนที่เปิด) · ถ้ามีอยู่แล้วคือ**รับช่วง** ไม่เปลี่ยนชื่อและไม่เปลี่ยนเจ้าของ · นี่คือจุดสร้างโปรเจกต์จุดที่สองนอกจาก `POST /api/projects` และเป็นเหตุผลที่ frontend เดิมยังทำงานได้โดยไม่ต้องแก้ (ดู [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) T-26)
- `project` มาด้วยเพื่อให้ client ที่เปิดโฟลเดอร์ตรง ๆ ได้ `id` ไปทำลิงก์ และได้ชื่อ/เจ้าของโดยไม่ต้องยิงซ้ำ

### `GET /api/image`
เสิร์ฟไฟล์ภาพดิบ

- **Query:** `path`
- **Response:** `FileResponse`
- **404** ถ้าไม่ใช่ไฟล์

### `GET /api/boxes`
คืนกล่องที่บันทึกไว้แล้วของภาพหนึ่งภาพ

- **Query:** `input_dir`, `image`, `kind` (`"pool"` default หรือ `"test"`)
- **Response:** `{"boxes": [Box...]}`

### `POST /api/label`
บันทึกป้าย: สกัด SAVPE embedding เข้า bank ต่อคลาส, mark ภาพว่า labeled, เขียนกล่องลง PostgreSQL (T-21)

- **Body:** `{"input_dir": str, "image": str, "boxes": [Box...], "model_id": str, "mode": "replace"|"update"}` (`mode` default `"replace"`, `model_id` default คือ `default_model` จาก `GET /api/config`)
- **Response:** `{"bank": BankSummary}`
- **400** ถ้าอ่านภาพไม่ได้, `boxes` ว่างเปล่า, หรือ `image` ถูกตั้งเป็น test set ไว้ (ข้อความ: `this image is in the test set -- it can never be taught to the model`) — กันไว้ตั้งแต่ endpoint เลยไม่ใช่แค่ฝั่ง UI เพราะ image ทดสอบกับ image ในพูลตอนนี้คือไฟล์เดียวกัน
- **409** ถ้า `model_id` ไม่ตรงกับโมเดลที่ bank นี้ล็อกไว้แล้ว (bank มี embedding อยู่ก่อนจาก checkpoint อื่น) — เกิดก่อนเรียก `extract_embedding()` เสมอ ไม่เสียเวลาโหลดโมเดลผิดตัว
- **พฤติกรรม:** `bank.lock_model(model_id)` ก่อน (ตั้งค่าให้ถ้า bank ยังไม่มีโมเดลเลย, ปฏิเสธถ้าไม่ตรง) → กล่องถูกจัดกลุ่มตามคลาสก่อน แล้วเรียก `extract_embedding()` **หนึ่งครั้งต่อคลาสต่อการบันทึก** (เฉลี่ยจากทุกกล่องของคลาสนั้นในภาพเดียวกัน) → `bank.add()` ต่อคลาส → `bank.mark_labeled()` → `bank.write_yolo_labels()` โดย `merge=True` เมื่อ `mode="update"`
- แต่ละ instance ใน bank บันทึก `labeled_by` = ผู้ใช้ที่ login อยู่ (FR-31) หรือ `null` เมื่อไม่มีระบบ login

### `POST /api/relabel`
เขียนไฟล์ label ของภาพใหม่โดยตรง **ไม่มีการสกัด embedding** — ใช้แก้ป้ายที่ auto-label/review

- **Body:** `{"input_dir": str, "image": str, "boxes": [Box...], "mode": "replace"|"update"}`
- **Response:** `{"bank": BankSummary}`
- **400** ถ้ามี `cls` ใดที่ยังไม่เคยเป็นคลาสใน bank เลย, หรือ `image` ถูกตั้งเป็น test set ไว้ (เหตุผลเดียวกับ `/api/label`)
- `boxes` เป็น list ว่างได้ (กรณี "โมเดลทำนายผิดทุกกล่อง" ก็ถือว่าถูกต้อง)

### `POST /api/predict`
กล่องที่โมเดลทำนายไว้สำหรับ **ภาพเดียว** ใช้เป็นกล่องร่างให้ผู้ใช้แก้แทนการวาดใหม่ (FR-19)

- **Body:** `{"input_dir": str, "image": str, "conf": float, "conf_by_class": {cls: float}}`
- **Response:** `{"boxes": [{"cls": str, "box": [...], "conf": float}]}`
- bank ว่าง → `{"boxes": []}` ทันที ไม่มี forward pass
- ไม่แตะ bank และไม่เขียนไฟล์ใด ๆ — กล่องที่คืนมาเป็นข้อเสนอ ยังไม่ใช่ป้าย
- ใช้โมเดลที่ล็อกไว้กับ bank เสมอ (`bank.model`) — **ไม่รับ `model_id` จาก client**, เหมือน `/api/score`, `/api/evaluate`, `/api/autolabel` ทั้งหมด

### `GET` / `POST` / `DELETE /api/history`
ประวัติผล evaluate เก็บที่ `<input_dir>/.ctflow/_bank/eval_history.json` (T-07) เก็บสูงสุด 200 จุด

- **GET/DELETE query:** `input_dir` · **POST body:** `{"input_dir": str, "point": {...}}`
- **Response:** `{"history": [point...]}` (ทุก method)

### `GET` / `POST /api/events`
สถิติความพยายามของผู้ใช้ (§7) เก็บที่ `<input_dir>/.ctflow/_bank/events.jsonl` แบบ append-only

- **POST body:** `{"input_dir": str, "kind": "session"|"label"|"fix"|"auto", "session": str, "secs": float|null, "written": int}` → `{"ok": true}`
- **GET query:** `input_dir` → `{"summary": {...}}` มี `sessions`, `sessions_reaching_autolabel`, `abandonment`, `manual_labels`, `median_label_secs`, `median_time_to_first_auto_secs`, `auto_written`, `corrections`, `correction_rate`
- ค่าที่ยังไม่มีข้อมูลเป็น `null` ไม่ใช่ `0` — "ยังไม่ได้วัด" กับ "วัดแล้วได้ศูนย์" ต่างกัน

---

## Test set (`internal/transport/httpapi/testset.go`, prefix `/api/testset`)

Ground truth สำหรับวัดผล ตั้งใจให้แยกขาดจาก prompt bank โดยสิ้นเชิง — **ไม่มีการคัดลอกไฟล์ภาพ**: test set คือภาพในพูลที่ถูก "แปะป้าย" ไว้เป็นแถวแยกใน PostgreSQL (`kind='testset'`, ดู `internal/infra/store`, T-21) ดังนั้น path ของภาพ test set กับภาพในพูลคือ path เดียวกันเป๊ะ ๆ ไม่มี `/api/testset/session` แยกอีกต่อไป — `POST /api/session` (`internal/transport/httpapi/pool.go`, `label.go`, `project.go`) คืน state ของ test set มาให้พร้อมกันแล้ว

### `POST /api/testset/import`
แปะป้ายภาพจากพูลว่าเป็น test set (ไม่คัดลอกไฟล์ — ข้ามภาพที่แปะป้ายอยู่แล้ว)

- **Body:** `{"input_dir": str, "images": [path...]}`
- **Response:** `{"images": [...], "labeled": [...], "classes": [...], "imported": [path ที่แปะป้ายจริง]}`
- แปะป้ายซ้ำภาพเดิมจะได้ `imported: []` (idempotent)

### `POST /api/testset/remove`
ถอดป้าย test set ออก + ลบ ground truth ของภาพนั้น (ไฟล์ภาพต้นฉบับในพูลไม่ถูกแตะต้องเลย — ไม่มีสำเนาให้ลบ)

- **Body:** `{"input_dir": str, "images": [path...]}`
- **Response:** `{"images": [...], "labeled": [...], "classes": [...], "removed": [path ที่ถอดป้ายจริง]}`

### `POST /api/testset/label`
เขียนป้าย ground truth ของภาพที่แปะป้ายเป็น test set ไว้แล้ว

- **Body:** `{"input_dir": str, "image": str, "boxes": [Box...], "mode": "replace"|"update"}`
- **Response:** `{"classes": [...], "labeled": [...]}`
- **400** ถ้าอ่านภาพไม่ได้, `boxes` ว่างเปล่า, หรือ `image` ยังไม่ได้แปะป้ายเป็น test set (ต้อง `/api/testset/import` ก่อน)

---

## Background jobs (`internal/transport/httpapi/jobs.go`)

การรัน inference รอบยาวทำงานผ่าน goroutine (`internal/platform/jobs`) — endpoint ที่สั่งงานคืน `job_id` ทันที ฝั่ง client ต้อง poll เอาเอง

### `GET /api/jobs/{job_id}`
ตรวจสถานะ job

- **Response:** job dict (`{done, total, started_at, finished, result, error}`) รวมกับ `{"now": <server time.time()>}`
- **404** ถ้าไม่รู้จัก `job_id`
- `ProgressBar.tsx` ใช้ `started_at`/`now` จาก server เพื่อคำนวณ ETA แทนการใช้นาฬิกาฝั่ง client (กัน clock skew)

### `POST /api/score`
รีสกอร์ภาพในพูลเทียบกับ bank ปัจจุบัน (background)

- **Body:** `{"input_dir": str, "images": [path...]}`
- **Response:** `{"job_id": str, "total": int}`
- ถ้า bank ว่างเปล่าหรือไม่มี path เลย job จะจบทันทีด้วย `result = {"scores": {}}`
- **ผลลัพธ์ (`result`):** `{"scores": {path: {"conf": float, "cls": str, "sig": [int × 64]}}}` — arm โมเดลครั้งเดียว แล้วรัน `predict_one(conf=0.05)` ต่อภาพ เก็บ detection ที่ confidence สูงสุด · `sig` คือ thumbnail 8×8 เทา ที่ UI ใช้กระจายลำดับภาพไม่ให้เสนอภาพคล้ายกันติดกัน (FR-18)

### `POST /api/evaluate`
ประเมิน bank ปัจจุบันเทียบ test set ที่มีป้ายแล้ว (background)

- **Body:** `{"input_dir": str, "conf": float, "conf_by_class": {cls: float}}` (`conf` default `0.25`)
- **Response:** `{"job_id": str, "total": int}`
- **400** ถ้า bank ว่างเปล่า หรือ test set ยังไม่มีป้าย (`metrics.load_ground_truth` โยน `FileNotFoundError`)
- **ผลลัพธ์ (`result`):** `metrics.evaluate(gt, pred)` รวมกับ `{"conf": conf, "conf_by_class": {...}}` — ดูรูปแบบเต็มในหัวข้อ `tools/metrics.py` ของ [ARCHITECTURE.md](./ARCHITECTURE.md)

### `POST /api/autolabel`
เขียนป้ายลง PostgreSQL ให้ภาพที่ระบุโดยตรงจาก bank (background)

- **Body:** `{"input_dir": str, "images": [path...], "conf": float, "conf_by_class": {cls: float}}` (`conf` default `0.25`)
- **Response:** `{"job_id": str, "total": int}`
- **400** ถ้า bank ว่างเปล่า
- **พฤติกรรม:** arm โมเดลครั้งเดียว → predict ทีละภาพ → เขียนไฟล์ label เฉพาะภาพที่มี detection (ไม่งั้นนับเป็น `no_detection`) → `bank.mark_auto()` เฉพาะภาพที่เขียนป้ายจริง
- **ผลลัพธ์ (`result`):** `{"written": int, "no_detection": int, "no_detection_images": [path...], "bank": BankSummary}` — คืนรายชื่อภาพที่ไม่เจออะไรเลย ไม่ใช่แค่จำนวน (FR-28)

### `POST /api/reembed`
เปลี่ยนโมเดลของ bank ที่ล็อกไปแล้ว โดย re-extract embedding ทุก instance ใหม่ด้วยโมเดลเป้าหมาย (background) — ดู FR-39

- **Body:** `{"input_dir": str, "model_id": str}`
- **Response:** `{"job_id": str, "total": int}` — `total` คือจำนวน instance รวมทุกคลาส (ไม่ใช่จำนวนภาพ)
- **400** ถ้า bank ยังไม่มีโมเดล (`bank.model is None` — ยังไม่เคยบันทึกกล่องแรกเลย ไม่มีอะไรให้ reembed), ถ้า `model_id` เท่ากับโมเดลปัจจุบันอยู่แล้ว, หรือ `model_id` ไม่อยู่ใน catalog
- **พฤติกรรม:** วน `bank.instances` ทุกคลาสทุก instance → อ่าน `source_image` ใหม่จากดิสก์ → `extract_embedding(img, [bbox], model_id)` ทีละตัว → เมื่อครบทุก instance แล้วค่อยเรียก `bank.reembed(model_id, new_embeddings)` ครั้งเดียวเพื่อ commit แบบ atomic (แทนที่ embedding ทั้งหมด + สลับ `bank.model` พร้อมกัน) — ถ้า job ล้มเหลวกลางทาง (เช่นภาพต้นทางถูกย้าย/ลบ) bank เดิมจะไม่ถูกแตะเลย เพราะ commit เกิดครั้งเดียวตอนจบเท่านั้น
- **ไม่แตะ:** PostgreSQL (labels, class order, image status), `bank.instances` (provenance) — เปลี่ยนแค่เวกเตอร์ใน `embeddings.pt` กับ `bank.model`
- **ผลลัพธ์ (`result`):** `{"bank": BankSummary}`

**สัญญาร่วมของ background job ทั้งสี่ตัว:** สร้างผ่าน `job_tracker.create(total)` → tick progress ทีละหน่วยงาน → `finish(result)` เมื่อสำเร็จ หรือ `fail(error)` เมื่อ exception — ฝั่ง frontend ใช้ `lib/api.ts`'s `runJob()` POST ไปเริ่ม job แล้ว poll `/api/jobs/{id}` ทุก 400ms จนกว่า `finished` · "arm โมเดล" ใน predict/score/evaluate/autolabel หมายถึง `vpe.arm(names, combined, bank.model_or_default)` เสมอ ไม่มี body ตัวไหนรับ `model_id` จาก client ไปกำหนดว่าจะ arm ด้วยโมเดลไหน — **ยกเว้น `/api/reembed`** ที่ `model_id` ใน body คือเป้าหมายที่ตั้งใจจะเปลี่ยนไปโดยตรง (ตรวจสอบแล้วว่าไม่ใช่ค่าเดิมและอยู่ใน catalog ก่อนเริ่ม job)

---

## Export (`internal/transport/httpapi/export.go` + `internal/core/export`, T-24)

ดาวน์โหลด annotation ของโปรเจกต์เป็น format ที่เลือกได้ อ่านตรงจาก PostgreSQL (`internal/infra/store`) ไม่ใช่ background job (ไม่มี inference, เร็วพอที่จะ synchronous ได้) — ไม่ใช้ตัวไหนแก้ state ทั้งสิ้น

### `GET /api/export`
- **Query:** `input_dir` (str), `format` (`"yolo"` default | `"coco"` | `"voc"`), `kind` (`"pool"` default | `"testset"`)
- **Response:** ไฟล์แนบ (`Content-Disposition: attachment`) — `application/zip` (yolo: `classes.txt` + `labels/*.txt`, voc: หนึ่ง XML ต่อภาพ) หรือ `application/json` (coco: `{images, annotations, categories}` เดียว)
- **400** ถ้า `format`/`kind` ไม่รู้จัก, หรือไม่มีอะไรให้ export (`kind` นั้นว่างเปล่า)
- พิกัดในตารางเป็น pixel อยู่แล้ว (ไม่เหมือน YOLO txt เดิมที่ normalize) — yolo/voc export ต้องเปิดภาพเพื่ออ่านขนาดตอนแปลงกลับเป็น normalized/แสดงใน XML เท่านั้น ภาพที่ถูกย้าย/ลบไปแล้วจะถูกข้าม ไม่ทำให้ export ทั้งก้อนล้มเหลว
