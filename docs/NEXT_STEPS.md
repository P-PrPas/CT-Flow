# CT-Flow — แผนงานถัดไป

> **สถานะ ณ 2026-08-24:** backend refactor เสร็จครบเฟส 0–3 และ merge เข้า `main` แล้วที่ commit `40b0c7f` จาก PR #2 ผลิตภัณฑ์ทำ workflow หลักได้ครบตั้งแต่ label → score → evaluate → auto-label → review → export
>
> เอกสารนี้เป็น active roadmap สำหรับงานหลัง refactor ส่วนรายละเอียดของ refactor อยู่ใน [REFACTOR_PLAN.md](./REFACTOR_PLAN.md)

## สถานะปัจจุบัน

- API เป็น Go และ inference/prompt bank เป็น Python sidecar
- PostgreSQL เก็บ annotation metadata และรองรับ concurrent labeling
- CI มี `go`, `python` และ `smoke` jobs และ merge commit ล่าสุดผ่านแล้ว
- ระบบเหมาะกับ PoC และทีมภายในที่ใช้ instance เดียว ยังไม่ใช่ production-scale deployment
- OIDC login แบบเดียวกับ `corpus-core` ทำครบทั้ง Go backend และ Next.js frontend แล้ว; local username/password ยังเป็น fallback

## หลักการจัดลำดับ

แก้สิ่งที่จำเป็นต่อผู้ใช้ก่อน ส่วนงาน scale หรือ algorithm ที่ยังไม่มีหลักฐานว่าจำเป็นให้รอ workload จริงและผลการทดลองก่อน ไม่เพิ่ม Redis, VRAM eviction หรือ NN-matching เพียงเพราะมีช่องว่างในเอกสาร

## Roadmap

### Phase 0 — Documentation และ deployment hygiene ✅

สถานะ: sync เอกสารหลักแล้วในรอบนี้ (2026-08-24); ownership check เป็น deployment preflight ทุกครั้งที่ใช้ `DATA_DIR` เดิม

- แก้เอกสารที่ยังอ้างชื่อไฟล์/CI job/สถานะก่อน merge
- ตรวจ `APP_UID` และ ownership ของ `DATA_DIR` ก่อน deploy เพราะข้อมูลเก่าที่เป็น root-owned จะเขียนไม่ได้เมื่อ container รันเป็น `app`
- ทำให้ README, PROJECT_STATUS และ requirements ใช้ชื่อ command และโครงสร้างปัจจุบันตรงกัน

เกณฑ์จบ: เอกสารที่ใช้ onboarding และ operation ระบุชื่อ command, CI และสถานะปัจจุบันตรงกับ repository และมี deployment checklist สำหรับข้อมูลเก่า

### Phase 1 — OIDC Login System ✅

สถานะ: ทำเสร็จ 2026-08-24 ตาม flow ของ `corpus-core`

- Authorization-code redirect และ server-side code exchange ผ่าน OIDC discovery
- ใช้ `sub` เป็น audit identity ที่เสถียร และแสดงชื่อจาก `preferred_username` → `email` → `sub`
- CSRF `state` อยู่ใน HttpOnly cookie และถูกตรวจแบบ constant-time ก่อน exchange
- provider token อยู่ฝั่ง Go เท่านั้น; browser ได้เฉพาะ application-session cookie อายุ 12 ชั่วโมง
- login/callback/logout UI, unauthenticated และ expired-session redirect ทำงานครบ
- `LABEL_TOOL_USERS` และ `/api/auth/login` เดิมยังใช้เป็น local fallback ได้
- integration test ใช้ mock OIDC provider ครอบ redirect, callback, claim mapping, session cookie และ state mismatch

### Phase 2 — Model quality: small-object classes

สถานะ: งาน product ถัดจาก OIDC เมื่อมี dataset และ acceptance criteria พร้อม

- ทำ T-08 crop-before-SAVPE เพื่อแก้ปัญหา `defect` ที่ยังได้ F1 ประมาณ 0.25 แม้ใช้ per-class threshold
- วัด precision/recall/F1 เทียบกับ baseline เดิมบน `conveyor_pvc`
- ถ้าผลทดลองชี้ว่าปัญหาคือ class variation จริง ค่อยทำ NN-matching bank เป็นงานแยก

ห้ามเริ่มด้วยการเปลี่ยน bank algorithm โดยไม่มีผลทดลองรองรับ เพราะ mean-pooling ยังเพียงพอสำหรับคลาสที่มีลักษณะสม่ำเสมอ

### Phase 3 — UX และ product completeness

ทำเมื่อมีผู้ใช้จริงยืนยันความต้องการ:

- Upload dropzone ที่เรียก `POST /api/upload` ผ่าน login
- ส่ง usage events จาก frontend ให้ข้าม session และข้ามเครื่องได้จริง
- เพิ่ม frontend type-check/build เข้า CI
- เพิ่ม export-format picker ใน UI หาก workflow ต้องใช้ COCO/VOC โดยตรง

### Phase 4 — Scale และ operations (conditional)

ทำเฉพาะเมื่อมี requirement รองรับ traffic หรือหลาย instance:

- ย้าย job tracker ไป Redis และเพิ่ม TTL/cleanup
- เพิ่ม VRAM eviction/LRU เมื่อการสลับหลาย checkpoint ทำให้เกิด OOM จริง
- เพิ่ม HTTPS/reverse proxy, deployment pipeline และ monitoring สำหรับ production

### Research backlog

ยังไม่ใช่งานถัดไปโดยอัตโนมัติ:

- active-learning selector ที่ใช้ uncertainty/diversity จริง
- retrain closed-set detector จาก annotation ที่สะสม

สองเรื่องนี้ควรเริ่มเมื่อมี baseline จากการใช้งานจริงและมีคำถามเชิงผลิตภัณฑ์ที่ชัดเจน

## Decision gates

- **OIDC:** เสร็จแล้ว; เพิ่ม role/workspace เฉพาะเมื่อมี policy จริง
- **Model quality:** ทำ crop experiment ก่อน NN-matching
- **Scale:** วัดจำนวนผู้ใช้, instance และ job concurrency ก่อนเพิ่ม Redis/eviction
- **Production:** กำหนด security, deployment และ observability requirements ก่อนเปิดใช้งานนอกทีมภายใน
