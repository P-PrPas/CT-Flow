# Label Tool — สถานะโปรเจกต์

## Test coverage

**หลัง port เป็น Go มีสามชั้น:** (1) `go test ./...` — unit test ต่อ package รวม `internal/infra/store` ที่รันกับ PostgreSQL จริง (2) `backend/tests/smoke_test.py` — end-to-end ยิงผ่าน HTTP จริง (3) `backend/tests/testdata/` — golden vector ข้ามภาษาที่ Go ต้อง reproduce ให้ตรงเป๊ะ

**smoke test ไม่ใช่ pytest** เป็นสคริปต์รันตรง (`SMOKE_BASE_URL=... python -m backend.tests.smoke_test`) ยิงผ่าน endpoint จริงทั้งหมด **ของ backend ตัวไหนก็ได้ที่ฟังอยู่ที่ URL นั้น** — นี่คือสิ่งที่ทำให้ "Go ทำงานเหมือน FastAPI" พิสูจน์ได้ด้วยคำสั่งเดียว ไม่ใช่ suite ที่สองที่เพี้ยนจากกันได้ · ตรวจสถานะ PostgreSQL ผ่าน `backend/tests/dbcheck.py` (คุยกับ DB ตรง ๆ ไม่ import โค้ดที่ตัวเองกำลังตรวจ) และตรวจ prompt bank ผ่าน `backend/tests/bank_test.py` แยกต่างหาก ครอบคลุมพฤติกรรมสำคัญต่อไปนี้ (T-21, 2026-08-21: อัปเดตให้ตรงกับ label storage ที่ย้ายไป PostgreSQL แล้ว — ทดสอบผ่านจริงกับ Postgres จริงในเซสชันนี้):

- session เปิดพบภาพครบ, label แล้ว bank summary/annotation ใน DB ตรงกับที่คาด
- bank (embedding) persist ข้าม instance ใหม่ได้จริง (โหลดซ้ำจากดิสก์แล้วค่าตรงกัน)
- `/api/score` ทำงานผ่าน background job + poll ได้ครบวงจร
- `mode="update"` รวมกล่องเดิม ส่วน `mode="replace"` (default) เขียนทับทั้งหมด — ใช้ได้ทั้ง `/api/label`, `/api/testset/label`, และ `/api/relabel`
- **ลำดับ index ของคลาสไม่เปลี่ยนเมื่อเพิ่มคลาสใหม่** แม้คลาสใหม่จะเรียงตามตัวอักษรมาก่อนคลาสเดิม (ทดสอบทั้งฝั่ง embeddings ใน `bank.py` และฝั่งตาราง `classes` ใน PostgreSQL โดยตรงผ่าน `_dbcheck.get_classes()`)
- **โมเดลที่ล็อกไว้กับ bank เปลี่ยนไม่ได้** — label ครั้งแรกล็อก `bank.model`, ส่ง `model_id` อื่นเข้ามาทีหลังได้ `409` ก่อนโหลดโมเดลผิดตัวด้วยซ้ำ (ทดสอบทั้งผ่าน HTTP และเรียก `Bank.lock_model()` ตรง ๆ)
- `/api/relabel` ไม่เพิ่ม embedding เข้า bank, ยอมรับ `boxes: []`, และปฏิเสธคลาสที่ไม่รู้จักด้วย `400`
- test set import **ไม่คัดลอกไฟล์ภาพ** — แค่แปะป้ายภาพในพูลเป็นแถวแยกใน DB (`kind='testset'`), import ซ้ำเป็น no-op, remove ไม่กระทบไฟล์ต้นทางในพูลหรือแถว `kind='pool'`
- **ภาพที่แปะป้ายเป็น test set ไม่มีทางถูกสอนเข้า prompt bank ได้เลย** — `POST /api/label`/`POST /api/relabel` ปฏิเสธด้วย `400` ยืนยันด้วย assertion ว่า ground truth แยกขาดจาก prompt bank จริง
- `/api/evaluate` และ `/api/autolabel` คืนโครงสร้างผลลัพธ์ตามที่คาด และสถานะ `auto` ใน DB sync กับจำนวนภาพที่เขียนป้ายจริง
- predict รอดจากการที่ bank เพิ่มคลาสกลางคัน (regression guard ของบั๊กจริงที่เคยเจอ — ดู [`vpe.py`](../backend/inference/vpe.py) หัวข้อ `arm()`)
- upload ปฏิเสธไฟล์ไม่ใช่ภาพ/เกินขนาด/ชื่อซ้ำ/ไม่มีชื่อ, path traversal ในชื่อไฟล์ถูกตัดทิ้งแทนที่จะพาไฟล์ออกนอกโฟลเดอร์
- auth: ไม่ login โดนปฏิเสธทุก endpoint ยกเว้น `/api/config`, รหัส/ชื่อผู้ใช้ผิดได้ `401` ทั้งคู่, login แล้วผ่าน, `labeled_by` ถูกบันทึกลง instance, logout แล้วกลับไป `401`

ฝั่ง Go มี `_test.go` ต่อ package: `config` (path safety 6 เคส รวม symlink หนีออกนอก root และ sibling ที่ชื่อขึ้นต้นเหมือน root), `auth` (เทียบกับ vector ที่ Python สร้าง), `store` (รวม concurrency test จริงกับ PostgreSQL — หลาย goroutine สร้างคลาสใหม่พร้อมกัน ยืนยันว่า class index ไม่ชนกัน, ดู [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md) หัวข้อ 4.1), `metrics`, `events`, `export`, `models`, `images`

ฝั่ง Python ที่เหลือมี self-check ของตัวเอง: `python -m backend.services.{models,metrics,groundtruth}`, `python -m backend.tests.dbcheck`, `python -m backend.tests.gen_testdata --check`

**ไม่มี** integration test ฝั่ง frontend, ไม่มีการวัด coverage เป็นตัวเลข

## CI/CD

`.github/workflows/backend.yml` มีสาม job ทั้งหมดรัน service container `postgres:16-alpine` (T-21) คู่กัน:

- **`go`** — `go vet`, `gofmt -l` (fail ถ้ามีไฟล์ไม่ format), `go test ./...` กับ postgres service, `go build`
- **`python`** — self-check ของ sidecar + `_gen_testdata --check` ไม่ต้องใช้ torch ไม่ต้องใช้ model weight จบในไม่กี่วินาที
- **`smoke`** — ติดตั้ง CPU torch (runner ไม่มี GPU), cache น้ำหนักโมเดล 28 MB ข้าม run, ยก sidecar + Go API ขึ้นจริง แล้วรัน `backend._smoke_test.py` ยิงผ่าน HTTP

ทดสอบแล้วว่า smoke test **fail จริง** เมื่อจงใจสลับ `Bank.classes` เป็น `sorted()` และ golden vector check **fail จริง** เมื่อจงใจขยับ IoU threshold เป็น 0.4 และ path-safety test **fail จริง** เมื่อเปลี่ยนเป็น `strings.HasPrefix` — ยืนยันว่า CI จับ regression ได้จริง ไม่ใช่แค่ผ่านเฉย ๆ ทริกเกอร์เมื่อ push/PR ที่แตะ `backend/**`, `cmd/**`, `internal/**`, `go.mod`

**ยังไม่มี** pipeline สำหรับ frontend (type-check/build เป็นการรันมือ `npx tsc --noEmit` / `npm run build`), ไม่มี auto-deploy เมื่อ merge

## Known bugs และข้อจำกัด (ยังไม่แก้ ณ ขณะเขียนเอกสารนี้)

- **Job tracker เก็บสถานะในหน่วยความจำ process เดียว.** ไม่ persist ข้าม restart, ไม่มี TTL/eviction ของ job เก่า — **port มาจาก `job_tracker.py` แบบไม่แก้พฤติกรรมโดยตั้งใจ** (`internal/platform/jobs`) มีคอมเมนต์ `ponytail:` ระบุทางแก้เป็น Redis/TTL ไว้แล้ว
- **Authentication เป็น opt-in, ปิดอยู่โดย default** — ไม่ตั้ง `LABEL_TOOL_USERS` เท่ากับไม่มีการยืนยันตัวตนเลย เหมือนพฤติกรรมเดิมทุกประการ backend (`/api/auth/*` + middleware) พร้อมแล้ว **แต่ยังไม่มีหน้า login บน UI** ต้องเรียก `POST /api/auth/login` เอง
- ~~**CORS เปิดกว้างทุก origin**~~ **หายไปแล้ว** — Go ไม่ตั้ง CORS header เลย ซึ่งถูกต้องเพราะ browser คุยผ่าน Next.js proxy อย่างเดียว ถ้าจะเปิด backend ให้เข้าถึงตรงในอนาคตต้องเพิ่มแบบระบุ origin
- **Bank ใช้ mean-pooling เดียวต่อคลาส** ไม่รองรับคลาสที่มี variation สูง (multi-modal) ได้ดี — โค้ดมีคอมเมนต์ชี้ทางอัพเกรดเป็น nearest-neighbor matching ไว้แล้วแต่ยังไม่ implement
- **คลาสขนาดเล็กที่ปะปนกับพื้นหลังยังแยกไม่ค่อยออก.** วัดจริงจาก dataset `conveyor_pvc` ที่ conf default เดิม (0.25): `defect` (รอยขีดข่วน/บิ่นเล็ก ๆ) recall = 0.00 ในขณะที่ `good_part` ได้ F1 0.82 — **T-01 พิสูจน์แล้วว่าสาเหตุหลักคือ threshold ไม่ใช่ไม่มีสัญญาณ** (recall ขยับเป็น 0.26 ที่ conf 0.05) `conf_by_class` ดึงทั้งสองคลาสมาพร้อมกันได้แล้ว (defect 0.248 + good_part 0.818) แต่ยังห่างจากเกณฑ์ auto-label (`READY_F1 = 0.75`) มาก — รายละเอียดเต็มที่ [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md)
- **ไม่มีปุ่มอัปโหลดไฟล์ในตัว UI** — backend (`POST /api/upload`) เสร็จและทดสอบแล้ว เหลือแค่ dropzone ฝั่ง frontend
- **ไม่มี Swagger UI แล้ว** — FastAPI แถมมาให้ ส่วน Go ไม่มี ตัดสินใจไม่ทำทดแทน เพราะ [API_REFERENCE.md](./API_REFERENCE.md) ละเอียดกว่า spec ที่เคย generate ได้ (ดู [REFACTOR_PLAN.md](./REFACTOR_PLAN.md) หัวข้อ 8 ข้อ 2)
- **โมเดลที่โหลดแล้วไม่เคยถูกปลดออกจาก VRAM** — `inference/vpe.py` แคชทุก `model_id` ที่เคยถูกเรียกใช้ไว้ตลอดอายุ process ไม่มี eviction สลับใช้หลายโมเดลขนาดใหญ่ในโปรเซสเดียวนานๆ จะสะสม VRAM โดยไม่มีเพดาน
- **ระวัง: เปลี่ยนจาก container ที่รันเป็น root มาเป็น `USER app` (non-root) แล้วทำให้ไฟล์เก่าที่มีอยู่แล้วใน `DATA_DIR` เขียนไม่ได้อีกต่อไป.** เจอจริงกับ `/data` ที่มีอยู่ก่อน — ไฟล์ root-owned ทั้งหมด ~4,377 รายการทำให้ `/api/label`, autolabel, และ session-open (หลัง FR-36 migration) พังด้วย `PermissionError` เงียบ ๆ (500 ที่ frontend เห็นเป็นแค่ "nothing happened" เพราะ response ไม่ใช่ JSON) แก้ครั้งเดียวด้วย `docker compose exec -u root api chown -R app:app /data` — ถ้า `DATA_DIR` ย้ายไปที่ใหม่หรือมีคนก็อปปี้ไฟล์เข้ามาด้วยเครื่องมืออื่นที่รันเป็น root (เช่น `docker cp` แบบไม่ใส่ `--user`) ปัญหานี้จะกลับมาอีก ไม่มี auto-remediation ในโค้ด ต้องรัน chown ซ้ำเอง
- ~~**หลายคนแก้ project เดียวกันพร้อมกันเสี่ยงเขียนชนกัน**~~ **แก้แล้ว (T-21, 2026-08-21)** — label/box storage ย้ายไป PostgreSQL, row lock ระดับ transaction แทน `filelock` ทั้งไฟล์ ดู [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md) · `docker compose build api` ของรอบนี้ยังไม่ได้ verify (ดูหัวข้อ "สถานะการยืนยัน build" ด้านล่าง)

## ความพร้อม deploy

**ใช้งานได้ในวง PoC/ทีมภายในที่ไว้ใจกันได้ ไม่ใช่ production พร้อมเปิดสาธารณะ**

- Authentication มีแล้วแต่เป็น opt-in (ปิดอยู่โดย default) — ถ้าไม่ตั้ง `LABEL_TOOL_USERS` ยังต้องพึ่งการควบคุมการเข้าถึงเครือข่ายภายนอกแอป (VPN, firewall, network segment) เหมือนเดิม และยังไม่มีหน้า login บน UI
- มี CI (`checks` + `smoke` job) แต่ไม่มี auto-deploy — การ deploy จริงยังเป็นการรัน `docker compose up --build` ด้วยมือ
- Job tracker แบบ in-memory จำกัดให้รันได้แค่ 1 uvicorn worker ต่อ instance
- ไม่มี HTTPS ในตัวแอป (certs mechanism มีไว้แค่ตอน build ผ่าน proxy องค์กรเท่านั้น)
- Container `api` ไม่รันเป็น root แล้ว (`ARG APP_UID` + `USER app`)
- ใช้ GPU (CUDA, `cu126`) เป็นค่าเริ่มต้นในการ build — override เป็น CPU ได้ด้วย build arg เดียวไม่ต้องแก้ไฟล์ (`--build-arg TORCH_INDEX_URL=.../whl/cpu`)
- เลือกโมเดล YOLOE ได้หลายเวอร์ชัน/ขนาด (11 ตัวเลือก) จากทุกที่ที่ตัวเลือกปรากฏ (Setup card หรือการ์ด "Model" ระหว่าง label) ไม่ใช่แค่ก่อนเปิด session ล็อกต่อ output folder ตั้งแต่กล่องแรกที่บันทึก — ไม่ต้อง redeploy เพื่อเปลี่ยนโมเดล · แต่ละตัวเลือกมีจุด 🟢/🔴 บอกว่ามี weight พร้อมใช้แล้วหรือยังต้อง auto-download ตอนใช้ครั้งแรก (`internal/platform/models`'s `IsAvailable()`, เช็คจาก `MODELS_DIR` จริง ไม่ cache ค่า) — pre-cache ไว้ 3 ตัว: `yoloe-11s-seg` (default), `yoloe-26s-seg`, `yoloe-26x-seg`
- **`/models` ใน Docker คือ named volume คนละไฟล์ระบบกับโฟลเดอร์ `label_tool/models/` บน host** — เคยลองแก้ด้วย `docker cp` เข้า container ที่รันอยู่ตรง ๆ ก่อน แต่พบว่าไม่รอด `docker compose down -v` (volume ถูกลบสร้างใหม่ว่างเปล่า) ตอนนี้แก้ถูกจุดแล้ว: `backend/Dockerfile` COPY ทั้งสามไฟล์เข้า image ที่ `/models/` ตรง ๆ ทำให้ volume ใหม่ (ครั้งแรกที่ `up` หรือหลัง `down -v`) auto-seed จาก image ตาม behavior ปกติของ Docker (volume ว่างเปล่าตอน mount ครั้งแรก = copy เนื้อหาจาก image ที่ mount point นั้นเข้ามาให้) — ทดสอบแล้วจริงด้วย `down -v` + `up` รอบใหม่ ยืนยันว่า `available: true` ทั้งสามตัวโดยไม่ต้องทำอะไรเพิ่ม (`.dockerignore` เปลี่ยนจาก exclude `models` ทั้งโฟลเดอร์ เป็น exclude `models/*` แล้ว negate ไฟล์ทั้งสามที่ต้องการ) ที่เหลือ auto-download จาก GitHub เข้า volume เมื่อถูกเลือกใช้ครั้งแรกเหมือนเดิม
- **ต้องตั้ง `POSTGRES_PASSWORD` ใน `.env`** (T-21) — `docker-compose.yml` ปฏิเสธ start ถ้าไม่ได้ตั้ง ดู `.env.example`

ข้อจำกัดเหล่านี้ล้วนเป็นงานวิศวกรรมที่ระบุสาเหตุและทางแก้ชัดเจนแล้ว (ดู [NEXT_STEPS.md](./NEXT_STEPS.md)) ไม่ใช่ความไม่แน่นอนเชิงสถาปัตยกรรม — เหมาะสำหรับขยายต่อเมื่อมีความต้องการรองรับผู้ใช้/traffic มากขึ้น

**สถานะการยืนยัน build:** `docker compose build api` รันสำเร็จจริงในเซสชันก่อนหน้า (ผ่านทุก step รวม `pip install` ด้วย cu126 wheel และ `useradd`/`chown` สำหรับ non-root) — เซสชันนี้ (T-21) **ยังไม่ได้ build image `api` ซ้ำเพื่อยืนยันว่า `psycopg2-binary` ที่เพิ่มใหม่ติดตั้งผ่านใน image จริง** ทดสอบ backend logic ทั้งหมดนอก Docker แทน (Python 3.13 + torch/torchvision CPU + `docker run postgres:16-alpine` จริง) — ยืนยันแค่ `docker compose config` (syntax ถูกต้อง) และเปิด service `db` เดี่ยว ๆ ผ่าน compose สำเร็จ ดู [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md) หัวข้อ 10
