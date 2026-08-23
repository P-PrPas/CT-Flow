# Label Tool — อภิธานศัพท์

คำอธิบายศัพท์เทคนิคที่ใช้ในเอกสารชุดนี้ เรียงตามลำดับที่มักพบเจอเมื่ออ่าน workflow ของเครื่องมือ ไม่ใช่ตามตัวอักษร

**YOLOE** โมเดล object detection แบบ zero-shot จาก Ultralytics ที่รับ "prompt" (ภาพตัวอย่างหรือข้อความ) แทนการเทรนคลาสตายตัวล่วงหน้า เครื่องมือนี้เลือกน้ำหนักได้จากหลายรุ่น/ขนาด (ดู "Checkpoint / model_id" ด้านล่าง), ค่า default คือ `yoloe-11s-seg.pt`

**Checkpoint / `model_id`** ไฟล์น้ำหนักของ YOLOE หนึ่งรุ่น/ขนาดที่เลือกได้จาก dropdown ตอนเริ่มโปรเจกต์ใหม่ (`backend/models.json` มีให้เลือก 11 แบบ ตั้งแต่ `yoloe-v8s-seg` เล็กสุด ถึง `yoloe-26x-seg` ใหญ่/แม่นสุด) **ล็อกกับ output folder ตลอดไปตั้งแต่กล่องแรกที่บันทึก** เพราะ embedding จากคนละ checkpoint ใช้แทนกันไม่ได้

**VPE (Visual Prompt Encoding)** กลไกของ YOLOE ที่แปลงตัวอย่างภาพ (visual prompt) ให้เป็น embedding แทนคลาสหนึ่ง ๆ แทนการสอนด้วยข้อความ

**SAVPE (Semantic-Aware Visual Prompt Encoding)** เวอร์ชันของ VPE ที่ YOLOE ใช้จริงในเครื่องมือนี้ ผ่าน `YOLOEVPSegPredictor.get_vpe()` — คือสิ่งที่ทำให้กล่องที่ผู้ใช้วาดกลายเป็นตัวเลข (embedding) ที่โมเดลนำไปเทียบกับภาพอื่นได้

**Prompt bank** ที่เก็บ embedding สะสมของทุกคลาสที่ผู้ใช้เคย label ไว้ด้วยมือ เก็บที่ `<input_dir>/.ctflow/_bank/` ยิ่งสะสมมาก โมเดลยิ่งแยกแยะคลาสนั้นได้แม่นขึ้น (implement เป็น class `Bank` ใน `inference/bank.py`)

**Mean-pooling** วิธีที่ bank ปัจจุบันสรุป embedding หลายตัวของคลาสเดียวกันให้เหลือตัวแทนเดียว — เอาค่าเฉลี่ยของทุก instance ในคลาสนั้น เป็นวิธีง่ายที่สุดที่ใช้งานได้ แต่ด้อยลงถ้าคลาสมีความหลากหลายสูง (ดู `mean_vpe()` ใน `bank.py`)

**Instance (ของ bank)** การ label หนึ่งกล่องหนึ่งครั้งที่ถูกแปลงเป็น embedding หนึ่งตัวและเก็บเข้า bank พร้อมที่มา (ภาพต้นทาง, ตำแหน่งกล่อง, เวลา)

**Pool (พูล)** ชุดภาพต้นทางทั้งหมดที่ยังไม่ label หรือ label ไปแล้วบางส่วน — คือ `input_dir` ที่เปิดผ่าน `/api/session`

**Rescore** การรันโมเดล (arm ด้วย bank ปัจจุบัน) กับภาพในพูลเพื่อดูค่าความมั่นใจ (confidence) โดยยังไม่เขียนป้ายจริง ใช้ตัดสินใจว่าจะ label ภาพไหนต่อ

**Test set / Ground truth** ภาพในพูลที่ถูกแปะป้ายไว้เป็นแถว `images` ที่ `kind='testset'` ใน PostgreSQL (ตั้งแต่ T-21 — ไม่คัดลอกไฟล์ภาพ) พร้อมป้ายที่มนุษย์วาดไว้เป็นคำตอบจริง **ไม่เคยถูกป้อนเข้า prompt bank** (backend ปฏิเสธด้วย `400` ถ้าใครพยายามส่งภาพที่แปะป้ายไว้เข้า `/api/label`) ใช้วัดผลเท่านั้น

**IoU (Intersection over Union)** สัดส่วนพื้นที่ทับซ้อนระหว่างกล่องสองกล่องหารด้วยพื้นที่รวม ใช้ตัดสินว่ากล่องที่โมเดลทำนายกับกล่อง ground truth "นับว่าตรงกัน" หรือไม่ — เครื่องมือนี้ใช้ threshold ที่ 0.5

**Precision / Recall / F1** ตัวชี้วัดคุณภาพการทำนายที่ IoU threshold คงที่ (ไม่ใช่ mAP): Precision = ทำนายถูกกี่ % ของที่ทำนายไป, Recall = จับของจริงได้กี่ % ของที่มีจริง, F1 = ค่าเฉลี่ยฮาร์มอนิกของสองตัวนี้ คือสัญญาณหลักที่ใช้ตัดสินใจว่า bank "พร้อมพอ" ให้ auto-label หรือยัง

**Auto-label** การให้โมเดลเขียนป้ายให้ภาพที่เหลือในพูลโดยอัตโนมัติจาก bank ปัจจุบัน (ผ่าน `/api/autolabel`) แยกบันทึกจากภาพที่ label ด้วยมือ

**Review mode** โหมดแก้ไขกล่องของภาพที่ auto-label ไปแล้ว — แก้ผ่าน `/api/relabel` ซึ่ง **ไม่สร้าง embedding ใหม่เข้า bank** เพราะถือเป็นการแก้ ไม่ใช่การสอนคลาสใหม่

**YOLO label format** รูปแบบไฟล์ป้ายมาตรฐาน: `labels/<stem>.txt` หนึ่งบรรทัดต่อกล่อง `<class_idx> <cx> <cy> <w> <h>` โดยพิกัดถูก normalize เป็น 0–1 เทียบกับขนาดภาพ

**`classes.txt`** ไฟล์ index → ชื่อคลาส (บรรทัดที่ N = index N) เป็น **append-only เท่านั้น** ห้ามเรียงใหม่หรือลบ เพราะไฟล์ label ทุกไฟล์อ้างอิงคลาสด้วยตำแหน่ง index นี้

**Background job** การประมวลผลที่ใช้เวลานาน (score/evaluate/autolabel/reembed) ซึ่งรันเป็น goroutine โดย endpoint ที่สั่งงานคืน `job_id` ทันที ฝั่ง frontend ต้อง poll `GET /api/jobs/{id}` เองจนกว่าจะเสร็จ

**`mode: local` vs `mode: vm`** ตั้งค่าผ่าน env `LABEL_TOOL_MODE` — `local` ยอมให้ browse ได้ทุก drive (ใช้ตอนรันบนเครื่องตัวเองนอก Docker), `vm` จำกัดการ browse ไว้แค่ใต้ `LABEL_TOOL_VM_ROOT` (ใช้เมื่อรันใน Docker บนเซิร์ฟเวอร์แชร์)

**`checkedPath()` / path safety** ตัวตรวจสอบกลางที่ทุก path จาก browser ต้องผ่านก่อนแตะดิสก์จริง ป้องกันไม่ให้ browser ขอเข้าถึงไฟล์นอกขอบเขตที่อนุญาต (โดยเฉพาะสำคัญใน `vm` mode) — resolve symlink แล้วเทียบเป็น path component ไม่ใช่ prefix ของ string

**`ponytail:`** convention คอมเมนต์ที่พบในโค้ดของ repo นี้ — ใช้ทำเครื่องหมายจุดที่ตั้งใจเลือกวิธีง่ายที่สุดที่ยังใช้งานได้ พร้อมระบุขีดจำกัดและแนวทางอัพเกรดถ้าจำเป็นในอนาคต (เช่นใน `bank.py`'s mean-pooling และ `internal/platform/jobs`'s in-memory storage)

**Inference sidecar (`vpe`)** service Python ที่เหลืออยู่หลัง port backend เป็น Go — ถือ YOLOE, torch และ prompt bank ไว้ทั้งหมด (`backend/inference/service.py`) API service ที่เป็น Go คุยกับมันผ่าน HTTP (JSON สำหรับงานสั้น, NDJSON สำหรับ inference pass ยาว ๆ เพื่อรายงาน progress ทีละภาพ) เหตุผลที่แยกไม่ได้: SAVPE head ไม่มีของเทียบเท่าใน Go และ `embeddings.pt` เป็น `torch.save` ดู [REFACTOR_PLAN.md](./REFACTOR_PLAN.md)

**Golden vector** ไฟล์ใน `backend/tests/testdata/` ที่บันทึกผลลัพธ์ของฟังก์ชัน pure ฝั่ง Python ไว้ (pbkdf2 hash, cookie ที่เซ็นแล้ว, ค่า F1, ไฟล์ COCO/VOC/YOLO) เพื่อให้ unit test ฝั่ง Go ต้อง reproduce ให้ตรงเป๊ะ — วิธีเดียวที่ใช้ได้จริงในการยืนยันว่าโค้ดสองภาษาให้คำตอบเดียวกัน ส่วนใหญ่ถูก "แช่แข็ง" แล้วเพราะ Python ที่สร้างมันถูกลบไปตอนจบ port

**Annotation storage (T-21)** ตั้งแต่ 2026-08-21 ป้าย/กล่อง/สถานะ label ทั้งหมด (สิ่งที่เคยเป็น `labels/*.txt`, `classes.txt`, `testset.json`) ย้ายจากไฟล์ไปตาราง PostgreSQL แล้ว (`internal/infra/store` ตั้งแต่ port เป็น Go, เดิมคือ `services/annotations_db.py`) เพื่อรองรับหลายคนแก้ project เดียวกันพร้อมกัน — **prompt bank (embedding) ไม่ย้าย ยังเป็นไฟล์เหมือนเดิม** เพราะเป็นคนละปัญหากัน (ดู [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md))

**Export** การดาวน์โหลด annotation ของโปรเจกต์เป็นไฟล์ในรูปแบบที่เลือกได้ (`GET /api/export`) — YOLO (กลับไปเป็น `labels/*.txt` + `classes.txt` แบบเดิม), COCO (JSON เดียว), หรือ Pascal VOC (XML ต่อภาพ) แทนที่การมีไฟล์ YOLO ติดอยู่บนดิสก์ตลอดเวลาแบบเดิมก่อน T-21
