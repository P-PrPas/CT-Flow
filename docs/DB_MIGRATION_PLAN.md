# Label Tool — แผนย้าย annotation storage ไป PostgreSQL + Export format selection

> **สถานะ: implement แล้ว (T-21–T-24 เสร็จ, T-25 เขียน config แล้วแต่ยังไม่ได้ build image จริง)** — ดูหัวข้อ 10 "สถานะการ implement จริง" ท้ายเอกสารสำหรับรายละเอียดว่าทำอะไรไปแล้วบ้าง ต่างจากแผนเดิมตรงไหน และทดสอบผ่านอะไรมาบ้าง เอกสารด้านล่างนี้คือแผนเดิมที่เขียนไว้ก่อนเริ่มงาน คงไว้เป็นบริบท ไม่ได้แก้ย้อนหลังทุกจุด
>
> **ที่มา:** งานแทรกที่ตกลงกับทีม (2026-08-21) แรงจูงใจหลักคือ (1) ต้องการรองรับหลายคนแก้ project เดียวกันพร้อมกัน โดยมีแผนทำระบบ login + workspace ในอนาคตแบบ Label Studio และ (2) ทีม infra อยากวาง DB เป็นรากฐานสำหรับอนาคต ส่วน scope ที่ตกลงกันคือ **ย้ายเฉพาะ label/box metadata (สิ่งที่ตอนนี้เป็น YOLO txt) ไปเป็นตาราง — `embeddings.pt` (prompt bank) ยังเป็นไฟล์เหมือนเดิม**
>
> **เอกสารที่เกี่ยวข้อง:** [ARCHITECTURE.md](./ARCHITECTURE.md) · [API_REFERENCE.md](./API_REFERENCE.md) · [REQUIREMENTS_STAKEHOLDER_ANALYSIS.md](./REQUIREMENTS_STAKEHOLDER_ANALYSIS.md) · [NEXT_STEPS.md](./NEXT_STEPS.md)

---

## 1. Scope

### ย้ายไป PostgreSQL
- `labels/<stem>.txt` ของพูล (`<input_dir>/.ctflow/labels/`)
- `labels/<stem>.txt` ของ test set (`<input_dir>/.ctflow/testset/labels/`)
- `classes.txt` ทั้งสองชุด (พูล + test set — **คนละ index space กัน**, ดูหัวข้อ 3)
- `testset.json` (manifest ว่าภาพไหนถูกแปะป้ายเป็น test set)
- `labeled` / `auto` (สถานะว่าภาพไหน label มือ/label โดยโมเดล) ที่ตอนนี้อยู่ใน `_bank/metadata.json`

### ยังเป็นไฟล์เหมือนเดิม (ไม่ย้าย)
- `_bank/embeddings.pt` — torch tensor ต่อ instance ไม่เหมาะกับตาราง relational, ไม่มี pain point อะไรที่ DB จะแก้ให้
- `_bank/metadata.json`'s `instances` (provenance ของแต่ละ embedding: source_image, bbox, added_at, labeled_by) และ `model` (model lock) — ผูกกับกลไก reembed (`Bank.reembed()`) และ `lock_model()` ที่ใช้ file lock อยู่แล้ว ไม่มีเหตุผลให้ย้าย
- `_bank/eval_history.json` — เบา, อ่านเป็น timeline ล้วน ไม่ต้อง query แบบ relational

**เหตุผลที่แบ่ง scope แบบนี้:** สิ่งที่ทีม infra ต้องการจริง ๆ คือแก้ปัญหา "หลายคนเขียน label พร้อมกัน" — นั่นคือ label/box metadata ล้วน ๆ ส่วน prompt bank (embedding) เป็นสถานะของโมเดลที่ผูกกับ `lock_model()`/`reembed()` อยู่แล้ว การย้ายพร้อมกันทั้งหมดจะเพิ่มความเสี่ยงโดยไม่ได้ประโยชน์เพิ่ม

---

## 2. Invariant เดิมที่ต้องรักษาไว้ให้ครบ

อ่านโค้ดจริงแล้ว (`services/bank.py`, `services/yolo_labels.py`, `services/groundtruth.py`) มี 3 กติกาที่ระบบพึ่งพาอยู่ตอนนี้ ต้องแปลไปเป็นกติกาของ DB ให้ตรงเป๊ะ ไม่งั้น dataset เก่าจะอ่านผิด:

1. **class index เป็น append-only เสมอ** — `labels/<stem>.txt` อ้างอิงคลาสด้วยตำแหน่ง (`<class_idx> <cx> <cy> <w> <h>`) ห้ามเรียงใหม่หรือลบเด็ดขาด (มี smoke test คุ้มครองอยู่แล้ว)
2. **พูลกับ test set มี class index คนละชุด** — `classes.txt` ของพูล (`<input_dir>/.ctflow/classes.txt`) กับของ test set (`<input_dir>/.ctflow/testset/classes.txt`) โตอิสระจากกัน คนละไฟล์ คนละลำดับ (`groundtruth.py::write_label` มี `classes.txt` ของตัวเอง)
3. **`Bank.classes` (`= list(self.embeddings.keys())`) ต้องยังคงเป็นลำดับเดียวกับ class index ของพูล** — เพราะ `mean_vpe()`/`set_classes()` และการเขียน label ไฟล์ใช้ลำดับนี้ร่วมกัน ถ้า class list ย้ายไป DB แต่ embeddings ยังเป็นไฟล์ ต้องมี**แหล่งความจริงเดียว**สำหรับลำดับนี้ — ดูหัวข้อ 4.3

---

## 3. Schema

```sql
-- รากฐานของแนวคิด "workspace" ในอนาคต — ตอนนี้แค่ 1 แถวต่อ input_dir หนึ่งโฟลเดอร์
CREATE TABLE projects (
    id          BIGSERIAL PRIMARY KEY,
    input_dir   TEXT NOT NULL UNIQUE,        -- path เดียวกับที่ browser ส่งมาวันนี้ (ผ่าน checked_path() เหมือนเดิม)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- แทน classes.txt — พูลกับ test set คนละ index space กัน (ดูข้อ 2.2) จึงต้องมี kind
CREATE TABLE classes (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    kind        TEXT NOT NULL,               -- 'pool' | 'testset'
    idx         INT NOT NULL,                -- 0-based, ตรงกับตำแหน่งบรรทัดใน classes.txt เดิม, ห้าม reuse/reorder
    name        TEXT NOT NULL,
    UNIQUE (project_id, kind, idx),
    UNIQUE (project_id, kind, name)
);

-- แทน labels/<stem>.txt ทั้งพูลและ test set (แยกด้วย kind) + testset.json (แถวที่ kind='testset' = "แปะป้ายเป็น test set แล้ว")
CREATE TABLE images (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    kind        TEXT NOT NULL,               -- 'pool' | 'testset'
    path        TEXT NOT NULL,               -- path เดียวกับไฟล์จริงบนดิสก์ (ไม่คัดลอก เหมือนพฤติกรรมเดิมของ test set)
    status      TEXT NOT NULL DEFAULT 'unlabeled',  -- 'unlabeled' | 'labeled' | 'auto'  (พูลเท่านั้นที่ใช้ auto)
    UNIQUE (project_id, kind, path)
);

-- แทนหนึ่งบรรทัดของ labels/<stem>.txt
CREATE TABLE annotations (
    id          BIGSERIAL PRIMARY KEY,
    image_id    BIGINT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    class_id    BIGINT NOT NULL REFERENCES classes(id),
    x1 REAL NOT NULL, y1 REAL NOT NULL, x2 REAL NOT NULL, y2 REAL NOT NULL,  -- pixel coords, ตรงกับ Box model ใน API_REFERENCE.md
    source      TEXT NOT NULL DEFAULT 'manual',   -- 'manual' | 'auto' (ใช้แยก /api/relabel ที่ไม่แก้ bank ออกจาก label มือ)
    created_by  TEXT,                              -- FR-31 (labeled_by) — เตรียมไว้เป็น FK จริงตอนมีระบบ user/workspace
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON annotations (image_id);
```

ไม่มี ORM — โค้ดในโปรเจกต์นี้ไม่มี dependency หนักที่ไหนเลย (`services/auth.py` ใช้ stdlib ล้วน, ไม่มี ORM ที่ไหนในโค้ดปัจจุบัน) ตาม convention เดิมของ repo แนะนำ driver เดียว (`psycopg[binary]` เวอร์ชัน 3, sync — backend เป็น sync FastAPI + `BackgroundTasks` อยู่แล้ว ไม่มี asyncio ที่ไหนเลย ไม่ต้องเปลี่ยนเป็น async driver) เขียน SQL ตรง ๆ ใน service ใหม่ (`services/annotations_db.py`) แทนไฟล์ `.txt` — ไม่ต้องมี migration framework (Alembic ฯลฯ) ด้วย เพราะ schema นี้เล็กและนิ่ง เขียน `schema.sql` ไฟล์เดียวรันตอน container start ก็พอ (`ponytail:` — เพิ่ม Alembic ตอนที่ schema เริ่มเปลี่ยนบ่อยจริง ไม่ใช่ตอนนี้)

---

## 4. รายละเอียดที่ต้องคิดให้ตกก่อนเริ่ม implement

### 4.1 Concurrency — เหตุผลที่แท้จริงที่ DB ช่วยได้

ปัญหาจริงของ multi-writer ไม่ใช่การเขียนไฟล์ชนกัน (`filelock` แก้เรื่องนั้นได้อยู่แล้ว) แต่คือ **race ตอนสร้างคลาสใหม่**: สองคนวาดกล่องคลาสใหม่คนละชื่อพร้อมกัน ต้องได้ index คนละค่าไม่ชนกัน วิธีทำให้ปลอดภัยจริงคือ lock แถว `projects` ก่อน แล้วค่อยคำนวณ `idx` ถัดไป ทั้งหมดในทรานแซกชันเดียว:

```sql
BEGIN;
SELECT id FROM projects WHERE id = $1 FOR UPDATE;   -- serialize ทุกคนที่แก้ project เดียวกัน
SELECT COALESCE(MAX(idx), -1) + 1 FROM classes WHERE project_id = $1 AND kind = $2;  -- next idx
INSERT INTO classes (project_id, kind, idx, name) VALUES ($1, $2, <next idx>, $3)
  ON CONFLICT (project_id, kind, name) DO NOTHING;   -- อีก transaction สร้างชื่อเดียวกันไปแล้วก็ไม่ error
COMMIT;
```

นี่คือ payoff จริงของงานนี้ — `FileLock` ทำแบบนี้ไม่ได้ดีเท่า DB transaction เพราะ lock ทั้งไฟล์ทั้งกระบวนการเขียน ในขณะที่ DB lock เฉพาะแถว project เดียว คนละ project ไม่ต้องรอกัน

### 4.2 `Bank.classes` ต้องมีแหล่งความจริงเดียว

วันนี้ `Bank.classes` (`services/bank.py:186`) มาจาก `list(self.embeddings.keys())` — ลำดับการ insert เข้า dict ของ embeddings เอง เมื่อ label/box ย้ายไป DB แต่ embeddings ยังเป็นไฟล์ ต้อง**เปลี่ยนให้ `Bank.classes` อ่านจากตาราง `classes` (kind='pool') แทน** ไม่ใช่จาก `embeddings.keys()` อีกต่อไป — ส่วนตัว dict `self.embeddings` ยังคง key ด้วยชื่อคลาส (ไม่ใช่ตำแหน่ง) เหมือนเดิม เพราะโค้ดที่เหลือ (`mean_vpe()`, `add()`, `reembed()`) เข้าถึงมันด้วยชื่อคลาสอยู่แล้วทุกจุด ไม่มีจุดไหนพึ่ง insertion order ของ dict โดยตรง — เปลี่ยนแค่ที่มาของ `classes` property ก็พอ ไม่ต้องแตะ logic อื่นใน `bank.py`

ผลคือ: การสอนคลาสใหม่ (`POST /api/label` เจอคลาสที่ไม่เคยมี) ต้อง get-or-create แถวใน `classes` (ตามข้อ 4.1) **ก่อน** เรียก `bank.add()` เพื่อให้ทั้งสองฝั่งเห็นคลาสเดียวกันในลำดับเดียวกันเสมอ

### 4.3 `bank.instances` (provenance สำหรับ reembed) ไม่ใช่สิ่งเดียวกับ `annotations`

ต้องแยกสองแนวคิดนี้ให้ชัด อย่ารวมเป็นตารางเดียวกันเด็ดขาด:

- **`annotations`** = กล่องหนึ่งกล่อง หนึ่งบรรทัดใน `labels/<stem>.txt` — กราฟ granularity ระดับ "กล่อง"
- **`bank.instances`** = การบันทึกครั้งหนึ่ง (ภาพ, คลาส) หนึ่ง embedding — ตาม `POST /api/label` ที่จัดกลุ่มกล่องตามคลาสก่อนแล้วเรียก `extract_embedding()` **หนึ่งครั้งต่อคลาสต่อการบันทึก** (เฉลี่ยจากทุกกล่องคลาสเดียวกันในภาพเดียว) — granularity ระดับ "การสอนโมเดลหนึ่งครั้ง" ซึ่งอาจครอบคลุมหลายกล่อง

ถ้าภาพหนึ่งมีกล่องคลาส `defect` สองกล่อง → ได้ 2 แถวใน `annotations` แต่ได้ embedding ใน bank แค่ 1 ตัว (เฉลี่ยจากสองกล่อง) — คนละอัตราส่วนกัน ห้ามพยายามให้ตารางเดียวตอบทั้งสองโจทย์ (ดู FR-39 ที่มีบั๊กแบบนี้อยู่แล้วฝั่ง reembed: instance ที่มาจากหลายกล่องคลาสเดียวกัน เก็บ bbox ตัวแทนได้แค่กล่องแรก — ปัญหานี้ยังคงอยู่หลัง migration เพราะ scope ตอนนี้ไม่แตะ `bank.instances`)

### 4.4 `labels/*.txt` จะไม่ใช่ "ของจริง" อีกต่อไป — เป็นการเปลี่ยน workflow ที่ต้องตัดสินใจ

`bank.py` docstring ปัจจุบันเขียนไว้ตรง ๆ ว่า `labels/<stem>.txt` คือ "the actual deliverable" — พอ DB กลายเป็นแหล่งความจริง ไฟล์ txt จะไม่ถูกเขียนสดอีกต่อไประหว่าง label (เว้นแต่จะจงใจ mirror ไปดิสก์ด้วย ซึ่งเพิ่มความซับซ้อนโดยไม่จำเป็นถ้ามี export endpoint แล้ว) **ข้อเสนอ: เลิก mirror ไฟล์ไปดิสก์ ให้ผลลัพธ์ (deliverable) มาจากปุ่ม Export เท่านั้น** (ดูหัวข้อ 6) ผู้ใช้ที่มี tooling เดิมที่อ่าน `labels/*.txt` ตรง ๆ จากโฟลเดอร์ต้องเปลี่ยนมา export ก่อน — เป็นความเสี่ยง breaking change ที่ต้องแจ้งทีมล่วงหน้า (ดูหัวข้อ 8)

---

## 5. Deployment

`docker-compose.yml` เพิ่ม service `db`:

```yaml
db:
  image: postgres:16-alpine
  environment:
    POSTGRES_DB: labeltool
    POSTGRES_USER: labeltool
    POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}   # ต้องมาจาก .env ไม่ hardcode
  volumes:
    - pgdata:/var/lib/postgresql/data
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U labeltool"]
```

`api` service เพิ่ม `depends_on: db (healthy)` + env var `DATABASE_URL` ใหม่ · volume `pgdata` ใหม่คู่กับ `models` ที่มีอยู่แล้ว

---

## 6. Export format selection (feature ที่ขอมาพร้อมกัน)

ไม่ต้องรอ DB migration เสร็จก็เริ่มได้ (ทำงานอิสระกัน) แต่ได้ประโยชน์เพิ่มหลัง migration เพราะ query จากตารางง่ายกว่า parse ไฟล์ txt:

- **`GET /api/export?input_dir=...&format=yolo|coco|voc&scope=pool|testset`**
- **YOLO** — zip ของ `labels/*.txt` + `classes.txt` (รูปแบบเดิมเป๊ะ ๆ คือ export กลับไปเป็นไฟล์แบบที่เคยมี)
- **COCO** — JSON เดียว (`images`, `annotations`, `categories`) แปลงพิกัดเป็น `[x,y,width,height]` ตาม COCO spec
- **Pascal VOC** — XML ต่อภาพ (`<annotation><object>...`)
- ทั้งหมด derive จาก query เดียวกัน (`images` JOIN `annotations` JOIN `classes` ของ project + kind ที่เลือก) — ตัว transformer ต่อ format เป็นฟังก์ชันล้วน (pure function: rows → bytes) ไม่มี state ไม่ต้องมี dependency ใหม่ (เขียน XML/JSON ด้วย stdlib พอ)
- ไม่ต้องมี format enum ขยายเป็น config ล่วงหน้า — เพิ่ม format ใหม่ค่อยเพิ่มฟังก์ชันแปลงใหม่ (YAGNI: 3 format ที่ระบุไว้ครอบคลุม use case ที่มีตอนนี้)

---

## 7. Roadmap ที่เกี่ยวโยงแต่ไม่อยู่ใน scope นี้

ทีมแจ้งว่ามีแผน login + workspace แบบ Label Studio ในอนาคต — งานนี้ (DB migration) **วางรากฐานให้** (ตาราง `projects` มี `id` ที่ future `users`/`memberships` table อ้างอิงได้, คอลัมน์ `annotations.created_by` พร้อมต่อยอด) แต่**ไม่ได้สร้างระบบ login/workspace จริงในรอบนี้** — auth ที่มีอยู่แล้ว (`services/auth.py`, opt-in ผ่าน `LABEL_TOOL_USERS`) ยังคงเดิมทุกประการจนกว่าจะมีงานแยกออกแบบ multi-tenant user model จริง (ต้องตัดสินใจเรื่อง roles, project ownership/sharing, ฯลฯ ซึ่งอยู่นอกขอบเขตเอกสารนี้)

---

## 8. ความเสี่ยง / คำถามที่ต้องตัดสินใจก่อนเริ่มงานจริง

| # | คำถาม | ผลกระทบถ้าไม่ตัดสินใจก่อน |
|---|---|---|
| Q1 | เลิก mirror `labels/*.txt` ไปดิสก์เลย หรือยังเขียนคู่ไปด้วย (DB = source of truth, ไฟล์ = cache แบบ read-only)? | ถ้ามี tooling ภายนอกอ่านไฟล์ตรง ๆ อยู่ตอนนี้ ต้องรู้ก่อนเริ่ม ไม่งั้นมันพังเงียบ ๆ วันที่ migration เสร็จ |
| Q2 | โปรเจกต์เก่า (เช่น `iron_ore`, `conveyor_pvc` ที่กล่าวถึงใน docs อื่น) ต้อง migrate เข้า DB ทันทีที่ deploy หรือ migrate แบบ lazy (โปรเจกต์ไหนถูกเปิดก่อนค่อย migrate)? | lazy migration ซับซ้อนกว่า (ต้อง detect ว่า project นี้ยัง file-based อยู่หรือ migrate แล้ว) แต่ downtime น้อยกว่า |
| Q3 | `DATABASE_URL` เดียวสำหรับทุก project หรือจะรองรับหลาย DB instance ในอนาคต (ตอน workspace จริงมาถึง)? | กระทบ connection pooling design ตั้งแต่วันแรก |
| Q4 | schema เวอร์ชันนี้ต้องรองรับ "หลาย workspace/org" เลยไหม หรือรอ phase login มาก่อน? | ถ้ารอ, ต้องมั่นใจว่า `projects.input_dir UNIQUE` ไม่ชนกับแนวคิด workspace ในอนาคต (คนละ org เปิด path ชื่อเดียวกันได้ไหม) |

**ข้อเสนอเริ่มต้น (default ถ้าไม่มีใครค้าน):** Q1 → เลิก mirror, ใช้ export endpoint เป็นทางเดียว (ง่ายกว่า ไม่มี state ซ้ำซ้อนให้ out-of-sync) · Q2 → migrate ทันทีตอน deploy ด้วย script ครั้งเดียว (โปรเจกต์ในระบบตอนนี้มีจำนวนน้อย ยังจัดการมือได้) · Q3/Q4 → DB เดียวพอสำหรับตอนนี้ ค่อยออกแบบ multi-tenant ตอนงาน workspace จริงเริ่ม (YAGNI)

---

## 9. ลำดับงานที่แนะนำ (ต่อจาก T-20 ในเอกสารหลัก)

#### T-21 · เขียน schema + service layer ใหม่ (`services/annotations_db.py`)
- **สิ่งที่ต้องทำ:** schema.sql ตามหัวข้อ 3, connection helper (`psycopg`), get-or-create class ตามหัวข้อ 4.1, ฟังก์ชันแทนที่ `yolo_labels.read_boxes`/`write_boxes` และ `groundtruth.py` ทั้งไฟล์ด้วย query ตรง ๆ
- **เกณฑ์ยอมรับ:** unit-level self-check (`python -m backend.services.annotations_db`) ตามสไตล์ `services/auth.py`/`services/metrics.py` — สร้าง project, สอนคลาสแข่งกัน (จำลอง concurrent) แล้วยืนยัน index ไม่ชนกัน

#### T-22 · เดินสาย router เดิมให้ใช้ DB แทนไฟล์
- **เชื่อมโยง:** `routers/pool.py` (`/api/label`, `/api/relabel`, `/api/boxes`), `routers/testset.py` ทั้งไฟล์
- **เกณฑ์ยอมรับ:** `_smoke_test.py` เดิมทั้งหมดผ่าน (แก้ assertion ที่เช็ค path ไฟล์ตรง ๆ ให้เช็ค DB แทนตามจำเป็น) — โดยเฉพาะ invariant เดิม: class index ไม่เปลี่ยนเมื่อเพิ่มคลาสใหม่, test set แยกขาดจาก bank

#### T-23 · Migration script สำหรับโปรเจกต์เก่า
- **สิ่งที่ต้องทำ:** อ่าน `.ctflow/classes.txt` + `.ctflow/labels/*.txt` + `.ctflow/testset/` ของทุกโฟลเดอร์ที่เคยเปิดผ่านระบบ แปลงเป็น insert เข้า DB โดยรักษาลำดับ index เป๊ะ
- **เกณฑ์ยอมรับ:** รันกับโปรเจกต์ทดสอบจริงที่มีอยู่ (เช่น `iron_ore`) แล้ว export กลับมาเป็น YOLO เทียบ md5sum กับไฟล์ต้นฉบับ (เนื้อหาต้องตรงกัน ลำดับบรรทัดต่างได้)

#### T-24 · Export endpoint (YOLO/COCO/VOC)
- **เชื่อมโยง:** หัวข้อ 6
- **เกณฑ์ยอมรับ:** export ทั้งสาม format จาก project เดียวกัน แล้วโหลดกลับด้วย library มาตรฐานของแต่ละ format (เช่น `pycocotools` อ่าน COCO JSON) ไม่ error

#### T-25 · docker-compose + deployment
- **เชื่อมโยง:** หัวข้อ 5
- **เกณฑ์ยอมรับ:** `docker compose up` จาก clean state ได้ DB พร้อม schema, `api` รอ `db` healthy ก่อน serve, ตัวแปร `POSTGRES_PASSWORD` มาจาก `.env` เท่านั้น (ไม่ commit ลง repo)

**ลำดับ:** T-21 → T-22 → T-23 (คู่ขนานกับ T-24 ได้ เพราะ export ไม่ผูกกับ migration script) → T-25 ปิดท้าย

---

## 10. สถานะการ implement จริง (2026-08-21)

T-21–T-24 ทำเสร็จและทดสอบผ่านจริงแล้ว (`_smoke_test.py` เต็มรูปแบบ รวม `/api/label`, `/api/relabel`, `/api/testset/*`, `/api/evaluate`, `/api/autolabel`, model lock, reembed, auth — ผ่านทั้งหมดกับ PostgreSQL จริงในเครื่อง + `services/annotations_db.py`, `services/db.py`, `_migrate_to_db.py` มี self-check ของตัวเองที่รันผ่านแล้วเช่นกัน) T-25 เขียน config ครบแล้ว (docker-compose.yml + .env.example + CI) แต่ **ยังไม่ได้ build image `api` จริงเพื่อยืนยัน runtime** — ตรวจแค่ `docker compose config` (syntax/interpolation ถูกต้อง) และเปิด service `db` เดี่ยว ๆ ผ่าน compose แล้วเห็น healthcheck เริ่มทำงาน

### สิ่งที่ต่างจากแผนเดิมที่เขียนไว้ก่อนเริ่มงาน (แก้ระหว่าง implement)

- **`Bank.classes` (`services/bank.py`) ไม่ต้องเปลี่ยนเลย** — หัวข้อ 4.2 ของแผนเดิมเข้าใจผิดว่า `Bank.classes` ต้องย้ายไปอ่านจาก DB เพื่อรักษา sync กับ label storage แต่พอไล่โค้ดจริงแล้วพบว่า `Bank.classes` (= `list(self.embeddings.keys())`) เป็นคำถาม **"บอทสอนคลาสนี้จาก embedding หรือยัง"** ล้วน ๆ (ใช้ตัดสิน `/api/relabel`'s unknown-class check และเป็น `names` ที่ป้อนเข้า `set_classes()`) ซึ่งเป็นคนละคำถามกับ "DB มีคลาสนี้ในตาราง label หรือยัง" — โดยธรรมชาติของ flow (`bank.add()` เพิ่ม embedding ก่อน แล้ว `annotations_db.write_boxes()` ค่อยสร้างแถวคลาสใน DB ทีหลังในคำขอเดียวกันเสมอ) DB pool classes จะเป็น subset ของ `bank.embeddings.keys()` เสมอ ไม่มีทางเห็นคลาสที่ DB มีแต่ embeddings ไม่มี — จึงไม่ต้องผูกสองระบบเข้าด้วยกัน ปล่อยให้ `bank.py` ไม่ต้องรู้จัก DB เรื่อง classes เลยก็ยังถูกต้อง (ยัง import `annotations_db` เพื่อใช้แค่ `list_by_status()` ใน `summary()` เท่านั้น)
- **ตัด column `annotations.source`** ('manual'/'auto') ออกจาก schema ที่ร่างไว้ตอนแรก — ไล่โค้ดเดิมแล้วพบว่าไม่มี endpoint ไหนอ่านค่าระดับกล่องแบบนี้เลย ระบบเดิม track แค่ระดับ "ภาพ" (`bank.labeled`/`bank.auto`) ซึ่งยังคงอยู่ (ย้ายเป็น `images.status`) — คอลัมน์นี้เป็นการเผื่ออนาคตที่ไม่มีใครขอ ตัดตาม YAGNI
- **`created_by`** เก็บไว้ (ต่างจาก `source`) เพราะผูกตรงกับเหตุผลหลักของงานนี้ (multi-user) และมี pattern เดิมรองรับอยู่แล้ว (`labeled_by` ใน bank instances, FR-31)
- **`classes.project_id` และ `images.project_id` ใส่ `ON DELETE CASCADE`** เพิ่มจากแผนเดิม (ตอนร่างลืมใส่ที่ตารางนี้) — จำเป็นสำหรับให้ `annotations.class_id ... ON DELETE CASCADE` ทำงานถูกต้องตอนลบทั้ง project (ใช้ใน test/migration cleanup, `annotations_db.delete_project()`)
- **driver: `psycopg2-binary` ไม่ใช่ `psycopg[binary]` (v3)** ตามที่แผนเดิมเสนอ — สภาพแวดล้อมที่ implement มี `psycopg2-binary` ติดตั้งไว้ล่วงหน้าอยู่แล้ว และ syntax แทบไม่ต่างกันสำหรับการใช้งานแบบ plain SQL ที่นี่ ไม่กระทบดีไซน์
- **`groundtruth.py` และ `yolo_labels.py` ไม่ได้ลบทิ้ง** ต่างจากที่คิดไว้ตอนแรกว่าจะกลายเป็น dead code — พบว่า `backend/_experiment_conf.py` (สคริปต์ทดลอง T-01) ยังพึ่ง `metrics.load_ground_truth()` (เวอร์ชันไฟล์เดิม) อ่านโฟลเดอร์ YOLO ดิบ (`data/conveyor_pvc/test`) ที่ไม่ใช่ `.ctflow` project เลย จึงคงฟังก์ชันเดิมไว้ครบ (ไม่แตะ) แล้วเพิ่ม `metrics.load_ground_truth_db(input_dir)` เป็นฟังก์ชันใหม่แยกต่างหากให้ `/api/evaluate` เรียกแทน — สองฟังก์ชันอยู่คู่กันโดยเจตนา ไม่ใช่ความสับสน
- **เลิก mirror `labels/*.txt`/`classes.txt`/`testset.json` ไปดิสก์ตามที่ Q1 เสนอไว้เป็นค่าเริ่มต้น** — ยืนยันแล้วว่าไม่มีใครค้าน ทำจริงตามนั้น: หลัง label ผ่าน `/api/label`/`/api/relabel`/`/api/testset/label` จะไม่มีไฟล์ label เขียนลงดิสก์อีกต่อไป มีแต่ `_bank/embeddings.pt` + `_bank/metadata.json` (prompt bank) เท่านั้นที่ยังเป็นไฟล์ ต้องใช้ `/api/export` เพื่อได้ไฟล์ YOLO/COCO/VOC กลับมา

### ไฟล์ที่เพิ่ม/แก้จริง

**ใหม่:** `backend/schema.sql`, `backend/services/db.py`, `backend/services/annotations_db.py`, `backend/routers/export.py`, `backend/_migrate_to_db.py`
**แก้:** `backend/services/bank.py` (ตัด `mark_labeled`/`mark_auto`/`write_yolo_labels`/`self.labeled`/`self.auto`, เพิ่ม `self.input_dir` + DB-backed `summary()`), `backend/services/metrics.py` (เพิ่ม `load_ground_truth_db`), `backend/routers/pool.py`, `backend/routers/testset.py` (เขียนใหม่ทั้งไฟล์), `backend/routers/jobs.py` (evaluate/autolabel), `backend/deps.py` (ตัด `test_dir()` ที่ไม่ใช้แล้ว), `backend/app.py` (startup hook เรียก `db.init_schema()`, เพิ่ม export router), `backend/_smoke_test.py`, `backend/requirements.txt`, `docker-compose.yml`, `.env.example`, `.github/workflows/backend.yml`

### สิ่งที่ยังไม่ทำ / ยังไม่ยืนยัน

- **`docker compose build api` ยังไม่ได้รันจริง** ในรอบ implement นี้ (torch cu126 build ใช้เวลานาน) — โค้ด backend ทดสอบผ่านนอก Docker แล้วด้วย Python 3.13 + torch/torchvision CPU + PostgreSQL จริง (`docker run postgres:16-alpine`) ไม่ใช่ผ่าน container `api` ที่ build จาก `backend/Dockerfile` โดยตรง — ความเสี่ยงที่เหลือคือเรื่อง build/packaging เท่านั้น (dependency ติดตั้งสำเร็จไหมใน image, non-root user เขียน schema ได้ไหม) ไม่ใช่ความถูกต้องของ logic
- **Frontend ไม่ถูกแตะเลย** ตามที่ตกลง scope ไว้ (backend/infra เท่านั้นในรอบนี้) — `/api/export` ยังไม่มีปุ่มเลือก format บน UI, และ endpoint response ทุกตัวมีรูปร่างเหมือนเดิมทุกประการ (ไม่ควรกระทบ frontend ที่มีอยู่แล้ว เพราะ contract ไม่เปลี่ยน) แต่ยังไม่ได้ทดสอบกับ frontend จริง
- **Migration script (`_migrate_to_db.py`) ยังไม่ได้รันกับโปรเจกต์เก่าจริง** (เช่น `iron_ore`, `conveyor_pvc` ที่กล่าวถึงในเอกสารอื่น) — ทดสอบแค่กับ fixture จำลองใน self-check เท่านั้น เพราะไม่มีโปรเจกต์เก่าเหล่านั้นอยู่ใน environment ที่ implement
