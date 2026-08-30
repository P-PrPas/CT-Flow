# CT-Flow — กู้ DB จาก `.ctflow` ด้วย `import_ctflow.py`

> **สถานะ: อ้างอิงการใช้งาน · เครื่องมืออยู่ที่ `backend/tools/import_ctflow.py`** — 2026-08-30
>
> **เอกสารที่เกี่ยวข้อง:** [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) ข้อ 8 · [API_REFERENCE.md](./API_REFERENCE.md) (`bank_orphaned`) · [GLOSSARY.md](./GLOSSARY.md) · [ARCHITECTURE.md](./ARCHITECTURE.md) "สถานะที่ถูกแบ่งกันอยู่คนละที่"

---

## 1. เมื่อไหร่ต้องใช้

สถานะของโปรเจกต์ถูกแบ่งอยู่สองที่ตั้งแต่ T-21:

| ที่เก็บ | เนื้อหา | ล้างด้วย |
|---|---|---|
| **PostgreSQL** (`pgdata` volume) | `classes` · `images` (labeled/auto) · `annotations` · test set | `docker compose down -v` |
| **`<input_dir>/.ctflow/_bank/`** (bind mount) | `embeddings.pt` · `metadata.json` (instances + model lock) | ลบโฟลเดอร์เอง |

`docker compose down -v` ล้างแค่ครึ่งแรก ถ้าไม่ได้ลบ `.ctflow/` ตามไปด้วย ผลคือ **โมเดลจำได้ว่าถูกสอนอะไร แต่ DB ไม่มีอะไรจำได้ว่าสอนจากภาพไหน** — UI จะขึ้นแถบเตือน *"The taught examples and the labels disagree"* (`bank_orphaned`, FR-51)

`import_ctflow.py` อ่าน `metadata.json` แล้วเขียน `classes` / `images` / `annotations` กลับเข้า DB สำหรับ `kind='pool'`

## 2. ก่อนใช้ — พิจารณาทางที่ lossless กว่า

ถ้ามี **`pg_dump` เก่า** อยู่ ให้ restore อันนั้นแทน — มันได้ครบทั้ง test set, ภาพ `auto`, และประวัติการแก้ ซึ่ง script นี้กู้ไม่ได้ (ดูข้อ 4)

```bash
# backup ทั้งสองครึ่งพร้อมกันเสมอ ก่อนทำอะไรกับ schema
docker exec ct-flow-db-1 pg_dump -U labeltool labeltool > backup_$(date +%F).sql
tar czf ctflow_$(date +%F).tgz -C "$DATA_DIR" \
  $(cd "$DATA_DIR" && find . -maxdepth 3 -name .ctflow -printf '%P\n')
```

`import_ctflow.py` ไว้ใช้ตอน **ไม่มี dump เหลือแล้ว** เท่านั้น

## 3. วิธีใช้

รันจาก repo root · ต้องมี `psycopg2` (`pip install -r backend/tests/requirements.txt`)

```bash
# 1. ดูก่อนว่าจะเขียนอะไร — ทำทุกอย่างแล้ว rollback
DATABASE_URL='postgresql://labeltool:<PW>@localhost:5433/labeltool' \
  python -m backend.tools.import_ctflow --input-dir /opt/mount/project/<dataset> --dry-run
```

```
bank:     /opt/mount/project/<dataset>/.ctflow/_bank
model:    yoloe-11s-seg
restores: 4 classes, 45 images, 97 boxes (pool)
classes:  ['red', 'yellow', 'green', 'blue']       ← ตรวจลำดับนี้ (ดูข้อ 5)
project:  1 (exists, empty)
```

```bash
# 2. โอเคแล้วค่อยลงจริง — ถาม y/N ก่อน commit (ข้ามด้วย --yes)
DATABASE_URL='postgresql://labeltool:<PW>@localhost:5433/labeltool' \
  python -m backend.tools.import_ctflow --input-dir /opt/mount/project/<dataset>
```

```bash
# self-check ของ logic การเรียงคลาส/แผ่กล่อง — ไม่ต้องใช้ DB
python -m backend.tools.import_ctflow --self-check
```

| flag | ความหมาย |
|---|---|
| `--input-dir` | โฟลเดอร์ที่มี `.ctflow/` อยู่ข้างใน — path เดียวกับที่ browser ส่ง (script ไม่แปลง) |
| `--name` | ชื่อ project ถ้าต้องสร้างใหม่ (default: ชื่อโฟลเดอร์) |
| `--owner` | `owner_oid` ถ้าต้องสร้างใหม่ (default: `NULL`) |
| `--dry-run` | ทำทั้งหมดใน transaction แล้ว rollback |
| `--yes` | ข้าม prompt ยืนยัน |

`<PW>` = `POSTGRES_PASSWORD` ใน `.env` · port `5433` มาจาก `docker-compose.override.yml`

## 4. กู้ได้ / กู้ไม่ได้

| | มาจากไหน |
|---|---|
| ✅ **classes** (`kind='pool'`) | key order ของ `metadata.json["instances"]` → `classes.idx` เริ่มที่ 0 |
| ✅ **images** `status='labeled'` | `instances[].source_image` (dedupe) |
| ✅ **annotations** | `instances[].bbox` + `labeled_by` → `created_by` + `added_at` → `created_at` |
| ✅ **model lock** | ไม่ต้องทำอะไร — ยังอยู่ใน `metadata.json` |
| ❌ **test set** (classes/images/annotations `kind='testset'`) | ภาพ test set สอน bank ไม่ได้ (invariant #3) → `_bank/` ไม่เคยมีข้อมูลนี้ · **F1 จะว่างจนกว่าจะสร้าง test set ใหม่มือเปล่า** |
| ❌ **ภาพ `status='auto'`** ที่ยังไม่ confirm | อยู่ใน DB อย่างเดียว — รัน Auto Label ใหม่ได้ |
| ⚠️ **กล่องที่ 2+ ของคลาสเดียวกันในภาพเดียว (ต่อ 1 teach action)** | `/vpe/teach` เก็บแค่ `cls_boxes[0]` ต่อคลาส (ข้อจำกัดเดียวกับ `reembed_stream`) |
| ⚠️ **การแก้ทีหลัง** | `bank.add()` append อย่างเดียว · `/api/relabel` ไม่แตะ bank → กล่องที่เคยสอนผิดแล้วแก้ จะกลับมาเป็นเวอร์ชันที่สอนไว้ |

## 5. ต้องตรวจอะไร

- **ลำดับคลาส** — `classes.idx` ต้องตรงกับลำดับใน `embeddings.pt` เป๊ะ (invariant #1) · script อ่านลำดับจาก `metadata.json` ซึ่งเป็น *ไฟล์ที่สอง* — ปกติ `Bank.add()` เขียนสองไฟล์ใต้ lock เดียวจึงตรงกัน แต่ crash ระหว่าง `_save()` คือสถานะครึ่ง ๆ ที่ script นี้เกิดมาเพื่อกู้พอดี · **เทียบ `classes:` ที่ script พิมพ์ กับ `classes` array จาก `POST /api/session` ของ bank นั้น ก่อนตอบ prompt**
  - ตรวจนอก API ได้ด้วย (ต้องมี torch): `torch.load('<...>/_bank/embeddings.pt', weights_only=False).keys()`
- **กล่องหลัง import** — เปิด Gallery แล้วดูด้วยตาว่าไม่มีกล่องซ้ำจากการ relabel เดิม
- **สร้าง test set ใหม่** — script พิมพ์เตือนตอนจบ

## 6. ทำไมไม่เป็น endpoint / ปุ่มใน UI

เขียน `classes.idx` แบบ append-only ผิดครั้งเดียว = ทำลายข้อมูลเงียบ ๆ (label ถัดไปพัง โดยไม่มี error) · script นี้ควรรันมือ ครั้งเดียว โดยมีคนดู — เหตุผลเดียวกับที่ repo ไม่มี reset script (PHASE2_WORKSPACE.md ข้อ 8) · guard ที่มี:

- ทำทั้งหมดใน **1 transaction** — ล้มกลางคัน = ไม่เขียนอะไรเลย
- **ปฏิเสธถ้า project มี pool class/image อยู่แล้ว** — รับเฉพาะเคส `bank_orphaned` (DB ว่าง) ไม่ merge
