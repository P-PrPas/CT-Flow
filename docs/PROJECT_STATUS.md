# Label Tool — สถานะโปรเจกต์

## Test coverage

**มี smoke test เดียว ไม่มี pytest suite.** `backend/_smoke_test.py` เป็นสคริปต์รันตรง (`python -m backend._smoke_test`) ไม่ใช่ pytest — ใช้ `TestClient` ยิงผ่าน endpoint จริงทั้งหมด รวมกับการสร้าง `Bank(...)` ตรง ๆ เพื่อตรวจสถานะบนดิสก์ ครอบคลุมพฤติกรรมสำคัญต่อไปนี้:

- session เปิดพบภาพครบ, label แล้ว bank summary/ไฟล์ label ตรงกับที่คาด
- bank persist ข้าม instance ใหม่ได้จริง (โหลดซ้ำจากดิสก์แล้วค่าตรงกัน)
- `/api/score` ทำงานผ่าน background job + poll ได้ครบวงจร
- `mode="update"` รวมกล่องเดิม ส่วน `mode="replace"` (default) เขียนทับทั้งหมด
- **ลำดับ index ของคลาสไม่เปลี่ยนเมื่อเพิ่มคลาสใหม่** แม้คลาสใหม่จะเรียงตามตัวอักษรมาก่อนคลาสเดิม (ทดสอบ invariant หลักของ `bank.py` โดยตรง)
- `/api/relabel` ไม่เพิ่ม embedding เข้า bank, ยอมรับ `boxes: []`, และปฏิเสธคลาสที่ไม่รู้จักด้วย `400`
- test set import เป็นการ**คัดลอก**ไฟล์ (ไม่ย้าย), import ซ้ำเป็น no-op, remove ไม่กระทบไฟล์ต้นทางในพูล
- **test dir ไม่มีโฟลเดอร์ `_bank/` เกิดขึ้นเลย** — assertion ตรงนี้คือการยืนยันว่า ground truth แยกขาดจาก prompt bank จริง
- `/api/evaluate` และ `/api/autolabel` คืนโครงสร้างผลลัพธ์ตามที่คาด และ `bank.auto` sync กับจำนวนภาพที่เขียนป้ายจริง

เพิ่มเติม `services/metrics.py` มี self-check เล็ก ๆ ของตัวเอง (`python -m backend.services.metrics`) ตรวจความถูกต้องของการคำนวณ IoU/TP/FP/FN ด้วยเคสมือ 3 กรณี

**ไม่มี** unit test แยกราย service, ไม่มี integration test ฝั่ง frontend, ไม่มีการวัด coverage เป็นตัวเลข

## CI/CD

**ไม่มี.** มี Dockerfile สำหรับทั้ง `backend` และ `frontend` แต่ไม่มี pipeline อัตโนมัติใด ๆ (ไม่มี GitHub Actions หรือเทียบเท่า) ที่รัน `_smoke_test.py`, build image, หรือ deploy อัตโนมัติเมื่อ push

## Known bugs และข้อจำกัด (ยังไม่แก้ ณ ขณะเขียนเอกสารนี้)

- **`/api/relabel` ไม่มี merge mode.** ต่างจาก `/api/label` และ `/api/testset/label` ที่มี `mode="update"` การแก้ป้าย auto-label เป็นการเขียนทับทั้งหมดเสมอ ผู้ใช้ต้องส่งกล่องครบทุกอันที่ต้องการเก็บไว้
- **Job tracker เก็บสถานะในหน่วยความจำ process เดียว.** ไม่ persist ข้าม restart, ไม่มี TTL/eviction ของ job เก่า (มีคอมเมนต์ `ponytail:` ระบุไว้ในโค้ดว่าต้องเปลี่ยนเป็น Redis หรือกลไก TTL ถ้าจะรองรับ multi-worker หรือผู้ใช้จำนวนมาก)
- **ไม่มี authentication/authorization ใด ๆ ทั้งระบบ** ใครก็ตามที่เข้าถึง URL ของแอปสามารถแก้ไข label, bank, และ test set ได้ทันทีโดยไม่มีการยืนยันตัวตน
- **Container `api` รันเป็น root** จำเป็นสำหรับเขียนลง bind mount `/data` ได้แน่นอนโดยไม่ต้องจัดการ UID ของ host แต่เป็นความเสี่ยงด้านความปลอดภัยถ้านำไปวางในสภาพแวดล้อมที่ไม่ไว้ใจได้เต็มที่
- **CORS เปิดกว้างทุก origin** (`allow_origins=["*"]`) — ยอมรับได้ในสถาปัตยกรรมปัจจุบันเพราะ FastAPI ถูกคุยด้วยผ่าน Next.js proxy เท่านั้น แต่ถ้ามีการเปิด backend ให้เข้าถึงตรงในอนาคตต้องทบทวนใหม่
- **Bank ใช้ mean-pooling เดียวต่อคลาส** ไม่รองรับคลาสที่มี variation สูง (multi-modal) ได้ดี — โค้ดมีคอมเมนต์ชี้ทางอัพเกรดเป็น nearest-neighbor matching ไว้แล้วแต่ยังไม่ implement
- **คลาสขนาดเล็กที่ปะปนกับพื้นหลังยังแยกไม่ค่อยออก.** วัดจริงจาก dataset `conveyor_pvc`: คลาส `defect` (รอยขีดข่วน/บิ่นเล็ก ๆ) ได้ F1 เพียง ~0.04–0.07 แทบไม่ขยับแม้เพิ่มจำนวน prompt ในขณะที่ `good_part` ทำได้ F1 ~0.78–0.80 จากแค่ 1–20 prompt — ยืนยันแล้วว่าไม่ใช่บั๊ก implementation (same-image prompt/predict score สูงถึง 0.95) แต่เป็นข้อจำกัดของ SAVPE ที่ความละเอียด 640px กับวัตถุขนาดเล็ก
- **ไม่มีปุ่มอัปโหลดไฟล์ในตัวแอป** — เป็นการตัดสินใจออกแบบ (ดู [PRODUCT_OVERVIEW.md](./PRODUCT_OVERVIEW.md)) แต่ทำให้ผู้ใช้ต้องมีทางเข้าถึงระบบไฟล์ของเซิร์ฟเวอร์ (network share/scp) อยู่แล้วก่อนจะใช้เครื่องมือนี้ได้

## ความพร้อม deploy

**ใช้งานได้ในวง PoC/ทีมภายในที่ไว้ใจกันได้ ไม่ใช่ production พร้อมเปิดสาธารณะ**

- ไม่มี authentication — ต้องพึ่งการควบคุมการเข้าถึงเครือข่ายภายนอกแอป (VPN, firewall, network segment) เท่านั้น
- ไม่มี CI/CD — การ deploy เป็นการรัน `docker compose up --build` ด้วยมือ
- Job tracker แบบ in-memory จำกัดให้รันได้แค่ 1 uvicorn worker ต่อ instance
- ไม่มี HTTPS ในตัวแอป (certs mechanism มีไว้แค่ตอน build ผ่าน proxy องค์กรเท่านั้น)
- Container `api` รันเป็น root

ข้อจำกัดเหล่านี้ล้วนเป็นงานวิศวกรรมที่ระบุสาเหตุและทางแก้ชัดเจนแล้ว (ดู [NEXT_STEPS.md](./NEXT_STEPS.md)) ไม่ใช่ความไม่แน่นอนเชิงสถาปัตยกรรม — เหมาะสำหรับขยายต่อเมื่อมีความต้องการรองรับผู้ใช้/traffic มากขึ้น
