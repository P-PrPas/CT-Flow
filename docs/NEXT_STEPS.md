# Label Tool — ข้อเสนองานต่อไป

เอกสารนี้สรุปช่องว่างระหว่างสิ่งที่ implement จริงกับเป้าหมายเดิม พร้อมรายการงานที่ควรพิจารณาทำต่อ เรียงตามความสำคัญโดยประมาณ **ไม่ใช่ requirements spec ฉบับสมบูรณ์** — เป็นจุดตั้งต้นสำหรับวางแผนงานรอบถัดไป ใช้คู่กับ [PROJECT_STATUS.md](./PROJECT_STATUS.md) และ [PRODUCT_OVERVIEW.md](./PRODUCT_OVERVIEW.md)

## สรุปสถานะปัจจุบัน → เป้าหมายที่ควรไปต่อ

เครื่องมือนี้ทำวงจร human-in-the-loop labeling จบ end-to-end แล้ว (label → bank → rescore → evaluate → auto-label → review) และมี smoke test ยืนยัน invariant สำคัญ (class-index stability, การแยกขาดของ test set กับ bank) แต่ยังเป็นเครื่องมือ PoC/ใช้ในทีมภายใน ไม่ใช่ระบบที่พร้อมขยายไปสู่การใช้งานจริงในสเกลใหญ่ หรือแทนที่การเทรน closed-set detector ตามที่เอกสารออกแบบเดิมตั้งเป้าไว้

## ช่องว่างเทียบกับเอกสารออกแบบเดิม (`../docs/yoloe-auto-label-tool-design.md`)

- **NN-matching bank** — เอกสารออกแบบเดิมตั้งใจให้ bank แยกแยะ variation ภายในคลาสเดียวกันด้วย nearest-neighbor matching ต่อ instance แต่ implementation จริงใช้ mean-pooling เฉลี่ยทุก instance ให้เหลือตัวแทนเดียวต่อคลาส (`bank.py`) — ใช้ได้ดีกับคลาสที่ลักษณะสม่ำเสมอ (เช่น `good_part` F1 ~0.78–0.80) แต่ยังไม่พิสูจน์ว่าจะช่วยแก้ปัญหาคลาสที่แยกยาก (เช่น `defect` F1 ~0.04–0.07) ได้จริงหรือไม่
- **Active-learning selector** — เอกสารออกแบบเดิมพูดถึงตัวเลือกภาพเชิงกลยุทธ์สำหรับ label ต่อ แต่ implementation จริงมีแค่การเรียง "confidence ต่ำสุดก่อน" ในหน้า UI ซึ่งเป็นตัวประมาณง่าย ๆ ไม่ใช่ตัวเลือกที่คำนวณจาก uncertainty/diversity จริง
- **Retrain closed-set detector** — เป้าหมายปลายทางของเอกสารออกแบบเดิมคือเมื่อสะสม label ได้มากพอ ให้ retrain โมเดล closed-set แยกต่างหาก แต่ยังไม่มีส่วนใดของโค้ดที่ทำสิ่งนี้ — ผลลัพธ์ปัจจุบันหยุดอยู่ที่ไฟล์ YOLO label เท่านั้น

## รายการงานที่ควรพิจารณาทำต่อ (เรียงตามความสำคัญโดยประมาณ)

1. **แก้ปัญหาคลาสขนาดเล็ก/แยกยาก (เช่น `defect`)** — T-01 ยืนยันแล้วว่าสาเหตุหลักคือ threshold ไม่ใช่ไม่มีสัญญาณ (recall 0.00 → 0.26 เมื่อลด conf) `conf_by_class` แก้ไปครึ่งทาง (defect F1 0.248) แต่ยังห่างจาก `READY_F1 = 0.75` — ขั้นต่อไปคือ crop เฉพาะพื้นที่สนใจก่อนส่งเข้า SAVPE (T-08 ในเอกสาร requirements) ดู [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md)
2. ~~เพิ่ม authentication/authorization ระดับพื้นฐาน~~ ✅ ทำแล้ว (`/api/auth/*` + middleware, ปิดอยู่จนกว่าจะตั้ง `LABEL_TOOL_USERS`) — เหลือหน้า login บน UI
3. **ย้าย job tracker ออกจากหน่วยความจำ process เดียว** — จำเป็นถ้าจะรองรับหลาย worker หรือผู้ใช้พร้อมกันจำนวนมาก (โค้ดมีคอมเมนต์ระบุแนวทางไว้แล้วว่าควรใช้ Redis/TTL eviction) — ยังไม่เกิด ยังไม่ทำ
4. ~~เพิ่ม `mode="update"` ให้ `/api/relabel`~~ ✅ ทำแล้ว
5. ~~ตั้ง CI พื้นฐาน~~ ✅ ทำแล้ว — `.github/workflows/backend.yml` (`checks` + `smoke` job) ยืนยันแล้วว่าจับ regression จริง (ทดสอบด้วยการสลับ `Bank.classes` เป็น `sorted()`)
6. **ประเมิน NN-matching bank อย่างจริงจัง** — ยังไม่ทำ รอ T-08 (crop) เสร็จก่อนตามลำดับใน requirements doc เพราะ T-01 ชี้ว่าปัญหาหลักคือ scale ไม่ใช่ diversity
7. ~~ทบทวนสิทธิ์ root ของ container `api`~~ ✅ ทำแล้ว — `ARG APP_UID` + `USER app`, build ยืนยันสำเร็จแล้ว
8. **พิจารณา active-learning selector ที่แท้จริง** — ถ้าปริมาณภาพที่ต้อง label ต่อโปรเจกต์ใหญ่ขึ้นมาก การเรียง "confidence ต่ำสุดก่อน" + thumbnail diversity แบบปัจจุบัน (FR-18) อาจไม่พอ ควรพิจารณา embedding distance จริงแทน thumbnail hash — ยังไม่ทำ
9. **GPU support** ✅ ทำแล้ว — `backend/Dockerfile` ติดตั้ง CUDA torch (`cu126`) เป็นค่าเริ่มต้น, `docker-compose.yml` ขอ GPU ผ่าน `deploy.reservations`, override เป็น CPU ได้ด้วย build arg เดียว
10. **เลือก YOLOE checkpoint ได้หลายเวอร์ชัน/ขนาด** ✅ ทำแล้ว — `services/models.py`, ล็อกต่อ output folder ผ่าน `Bank.lock_model()`, ดู FR-36 ใน [REQUIREMENTS_STAKEHOLDER_ANALYSIS.md](./REQUIREMENTS_STAKEHOLDER_ANALYSIS.md)
11. **ลบกล่อง pre-annotation ที่โมเดลทำนายเกินได้ทีละกล่อง** ✅ ทำแล้ว — `BoxCanvas.tsx` (`onRemoveDraft`), ดู FR-37 ใน [REQUIREMENTS_STAKEHOLDER_ANALYSIS.md](./REQUIREMENTS_STAKEHOLDER_ANALYSIS.md)
12. **เลือกโมเดลได้ทุกที่ที่ตัวเลือกปรากฏ ไม่ใช่แค่ตอนตั้งค่าก่อนเปิด session + จุดบอกสถานะ weight พร้อมใช้** ✅ ทำแล้ว — แยก `ModelPicker.tsx` ออกมาใช้ร่วมกันระหว่าง Setup card กับการ์ด "Model" ในหน้า label · เพิ่ม `available: bool` ต่อโมเดลใน `GET /api/config` (`services/models.py::is_available()`) พร้อมจุด 🟢/🔴 ใน UI · pre-cache `yoloe-26s-seg` และ `yoloe-26x-seg` ไว้ใน `label_tool/models/` ควบคู่กับ default แล้ว (ก่อนหน้านี้มีแค่ default ทำให้เลือก v26 แล้ว predict เงียบล้มเหลวเพราะ auto-download ไปไม่ถึงปลายทาง) ดู FR-36/FR-38
13. **เปลี่ยนโมเดลของโปรเจกต์ที่ล็อกไปแล้วได้จริง (re-embed) โดยไม่ต้องเริ่ม output folder ใหม่** ✅ ทำแล้ว — `Bank.reembed()` + `POST /api/reembed` (background job) วนอ่าน `bank.instances` ทุกตัวย้อนกลับไปที่ภาพต้นทางเดิม รัน embedding ใหม่ด้วยโมเดลเป้าหมาย แล้ว commit แทนที่ทั้งชุดพร้อมสลับ `bank.model` แบบ atomic · ไม่แตะ `labels/*.txt` เลย (ยืนยันด้วย md5sum ตรงกันก่อน/หลังในการทดสอบจริง) · ปุ่ม "Switch model…" ใน `ModelPicker.tsx` ดู FR-39

## ความเสี่ยง / คำถามที่ทีมควรตัดสินใจก่อนเริ่มงานต่อ

- ถ้าจะแก้ปัญหาคลาส `defect`: จะเพิ่มความละเอียดภาพหรือ crop ก่อน มีผลกับ throughput ของ inference มากแค่ไหน — ยังไม่มีข้อมูลวัดผล ต้องทดลองก่อนตัดสินใจ
- ถ้าจะเปิดให้หลายคนใช้งานพร้อมกัน: จะรองรับหลาย session เขียน bank เดียวกันพร้อมกันจริง ๆ หรือจำกัดให้ 1 คนต่อ 1 โปรเจกต์ (`input_dir`) เหมือนเดิมแต่เพิ่มแค่ auth ชั้นนอก — ยังไม่มีการตัดสินใจเรื่องนี้ในโค้ดปัจจุบัน
- การ retrain closed-set detector จากป้ายสะสม: ยังไม่มีการยืนยันว่าปริมาณ label ที่สะสมได้จากทีมภายในเพียงพอต่อการ retrain ที่ให้ผลดีกว่าใช้ prompt bank ต่อไปหรือไม่ — ควรทดลองเปรียบเทียบก่อนลงทุนสร้างระบบ retrain
