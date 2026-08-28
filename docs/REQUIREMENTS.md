# CT-Flow — Requirements & Stakeholder Analysis

> **วัตถุประสงค์ของเอกสารนี้:** ทะเบียน requirement รายข้อพร้อมสถานะ — ตอบคำถาม *"ต้องทำอะไรบ้าง ผู้ใช้ถึงจะ label ได้ง่าย เร็ว และไม่น่าเบื่อ"* และ *"ข้อ X ทำแล้วหรือยัง"*
>
> **ลำดับงานไม่ได้อยู่ในเอกสารนี้** — [ROADMAP.md](./ROADMAP.md) คือ source of truth ของลำดับงาน และ [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) คือแผนของงานที่กำลังทำอยู่
>
> **เอกสารที่เกี่ยวข้อง:** [PRODUCT_OVERVIEW.md](./PRODUCT_OVERVIEW.md) · [ARCHITECTURE.md](./ARCHITECTURE.md) · [API_REFERENCE.md](./API_REFERENCE.md) · [ROADMAP.md](./ROADMAP.md) · [GLOSSARY.md](./GLOSSARY.md) · [history/EXPERIMENT_T01_CONF.md](./history/EXPERIMENT_T01_CONF.md)

---

## สารบัญ

1. [หลักการชี้นำ: ทำไม "ไม่น่าเบื่อ" ถึงเป็น requirement ไม่ใช่ nice-to-have](#1-หลักการชี้นำ)
2. [Stakeholder analysis](#2-stakeholder-analysis)
3. [User personas และ pain point](#3-user-personas-และ-pain-point)
4. [User journey ปัจจุบัน + จุดที่เกิด friction](#4-user-journey-ปัจจุบัน--จุดที่เกิด-friction)
5. [Functional requirements (แยกสถานะ)](#5-functional-requirements)
6. [Non-functional requirements](#6-non-functional-requirements)
7. [Success metrics — วัดว่า "ง่ายและไม่น่าเบื่อ" สำเร็จหรือยัง](#7-success-metrics)
8. [สมมติฐานที่ต้องตรวจสอบก่อนลงทุนงานใหญ่](#8-สมมติฐานที่ต้องตรวจสอบ)

---

## 1. หลักการชี้นำ

### ทำไมเรื่อง "ง่ายและไม่น่าเบื่อ" ถึงเป็น requirement ระดับแกน

การ label ภาพเป็นงานที่ล้มเหลวจาก **ความเหนื่อยล้าของมนุษย์** บ่อยกว่าล้มเหลวจากข้อจำกัดทางเทคนิค เมื่อผู้ใช้เบื่อ ผลลัพธ์ที่ตามมาไม่ใช่แค่ "ทำช้าลง" แต่คือ:

- กล่องเริ่มวาดหยาบขึ้น → embedding คุณภาพต่ำเข้า bank → โมเดลแย่ลง → ต้อง label เพิ่ม → เบื่อกว่าเดิม (วงจรถดถอย)
- ผู้ใช้กด auto-label เร็วเกินไปเพราะอยากให้จบ ทั้งที่ F1 ยังไม่พอ → ได้ dataset คุณภาพต่ำที่ต้องมาแก้ทีหลัง (แพงกว่าเดิม)
- ผู้ใช้เลิกใช้เครื่องมือกลางทาง → prompt bank ที่สะสมไว้สูญเปล่า

**หลักการออกแบบที่ใช้ตัดสินใจทุกข้อในเอกสารนี้:**

| หลักการ | ความหมายเชิงปฏิบัติ |
|---|---|
| **P1 — ทุกคลิกต้องคุ้มค่า** | ถ้าผู้ใช้ต้อง label 1 ภาพ ภาพนั้นต้องเป็นภาพที่ให้ information สูงสุดที่ระบบเลือกให้ได้ ไม่ใช่ภาพถัดไปตามลำดับไฟล์ |
| **P2 — ผู้ใช้ต้องเห็นความคืบหน้าตลอดเวลา** | ความรู้สึกว่า "ที่ทำไปมีผล" คือสิ่งที่กันความเบื่อได้ดีที่สุด — ต้องเห็นว่า label แล้วโมเดลดีขึ้นแค่ไหน |
| **P3 — ระบบต้องบอกว่าเมื่อไหร่ควรหยุด** | ความเบื่อสูงสุดเกิดตอนไม่รู้ว่าต้อง label อีกกี่ภาพ — ระบบต้องตอบให้ได้ว่า "พอหรือยัง" |
| **P4 — งานซ้ำซากต้องถูกทำให้เหลือคลิกเดียว** | keyboard shortcut, การจำคลาสล่าสุด, การ copy กล่องจากภาพก่อนหน้า |
| **P5 — ห้ามให้ผู้ใช้ทำงานที่เสียเปล่า** | เช่น การแก้ auto-label ที่ไม่ช่วยสอนโมเดล ต้องบอกให้ผู้ใช้รู้ ไม่ใช่ปล่อยให้แก้ไปเรื่อยๆ โดยไม่มีผล |

---

## 2. Stakeholder Analysis

| Stakeholder | บทบาท | สิ่งที่ต้องการจริง | ความเสี่ยงถ้าไม่ตอบสนอง | ระดับความสำคัญ |
|---|---|---|---|---|
| **ML/CV Engineer** (ผู้ใช้หลักปัจจุบัน) | ใช้เครื่องมือสร้าง dataset สำหรับงาน detection | ได้ labeled dataset คุณภาพพอเทรนโมเดล เร็วกว่าการ label มือล้วน | กลับไปใช้ CVAT/Label Studio แบบเดิม เครื่องมือถูกทิ้ง | **สูงสุด** |
| **QC Operator / Domain expert** (ผู้ใช้เป้าหมายอนาคต) | รู้จริงว่าอะไรคือ defect แต่ไม่ใช่วิศวกร | UI ที่ใช้ได้โดยไม่ต้องเข้าใจ embedding/SAVPE, ไม่ต้องแตะ filesystem | ความรู้ domain ไม่ถูกดึงเข้าระบบ ต้องพึ่ง engineer ตลอด | **สูง** (ยังไม่รองรับ) |
| **Tech Lead / หัวหน้าทีม** | ตัดสินใจว่าจะลงทุนต่อหรือไม่ | เห็นว่า ROI ชัด (ลดเวลา label กี่ %), เห็น risk ที่ควบคุมได้ | โปรเจกต์ถูกตัดงบก่อนพิสูจน์คุณค่า | **สูง** |
| **ผู้ใช้ปลายทางของ dataset** (คนเทรนโมเดล production) | รับ output ไปเทรน closed-set detector | Label format ถูกต้อง, class index เสถียร, รู้ว่าภาพไหน manual/auto | Dataset ใช้ไม่ได้ ต้องตรวจใหม่ทั้งหมด | **สูง** (ตอบสนองแล้ว) |
| **IT / Infra** | ดูแลเซิร์ฟเวอร์ที่ deploy | ความปลอดภัยพื้นฐาน, ไม่กินทรัพยากรเกินควบคุม | ไม่อนุมัติให้ deploy นอก sandbox | **กลาง** (ยังมีช่องว่าง: auth, root container) |

### ข้อสังเกตสำคัญเรื่อง stakeholder

**ปัจจุบันเครื่องมือรองรับเฉพาะ ML Engineer เท่านั้น** — auth มีครบแล้ว (Phase 1) แต่การไม่มีปุ่มอัปโหลดยังทำให้ QC Operator (คนที่รู้จริงที่สุดว่าอะไรคือ defect) เข้ามาใช้เองไม่ได้ ต้องมีคนเอาภาพไปวางบนเซิร์ฟเวอร์ให้ก่อนเสมอ

นี่คือ **คอขวดเชิงองค์กร ไม่ใช่แค่เชิงเทคนิค**: คนที่มีความรู้ domain มากที่สุดคือคนที่ label ได้ถูกต้องที่สุด แต่กลับเข้าถึงเครื่องมือไม่ได้ ถ้าเป้าหมายระยะกลางคือขยายการใช้งานในองค์กร ข้อนี้สำคัญกว่างาน optimization หลายข้อ

---

## 3. User Personas และ Pain Point

### Persona A — "เอก" ML Engineer (ผู้ใช้หลักตอนนี้)

- **บริบท:** ได้รับ dataset ภาพสายพาน 3,000 ภาพ ต้องส่งมอบ labeled dataset ภายใน 2 สัปดาห์
- **ความคาดหวัง:** ไม่ต้อง label ครบ 3,000 ภาพด้วยมือ — label สัก 100-200 ภาพแล้วให้ระบบทำที่เหลือ
- **Pain point ปัจจุบัน:**
  - ไม่รู้ว่าต้อง label อีกกี่ภาพถึงจะพอ ต้องกด Rescore/Evaluate เดาเอง
  - คลาส `defect` ไม่ว่าจะ label เท่าไหร่ F1 ก็ไม่ขยับ (0.04–0.07) แต่ระบบไม่บอกว่า "คลาสนี้ตันแล้ว หยุดเถอะ" ปล่อยให้ label ต่อไปเรื่อยๆ โดยเสียเปล่า
  - การแก้ auto-label ใน review mode ไม่ช่วยให้โมเดลดีขึ้น แต่ UI ไม่ได้สื่อสารเรื่องนี้

### Persona B — "แนน" QC Operator (ผู้ใช้เป้าหมายที่ยังเข้าไม่ถึง)

- **บริบท:** ทำงานหน้าไลน์ผลิต รู้ทันทีว่าชิ้นไหนมีตำหนิ แต่ไม่รู้จัก YOLO/embedding
- **ความคาดหวัง:** เปิดเว็บ ลากไฟล์ภาพเข้ามา วาดกรอบ กด save จบ
- **Pain point ปัจจุบัน:** ใช้ไม่ได้เลย — ต้องมีคนเอาภาพไปวางใน `/opt/mount/project` ให้ก่อน และคำศัพท์บนหน้าจอ (bank, embedding, rescore) ไม่สื่อความหมายกับเธอ

### Persona C — "โจ" Tech Lead

- **ความคาดหวัง:** ตัวเลขที่พิสูจน์ว่าเครื่องมือนี้ประหยัดเวลาจริงเทียบกับ label มือ
- **Pain point ปัจจุบัน:** ระบบไม่เก็บสถิติเวลา/จำนวนที่ประหยัดได้เลย ตอบคำถาม "คุ้มไหม" ด้วยข้อมูลจริงไม่ได้

---

## 4. User Journey ปัจจุบัน + จุดที่เกิด Friction

```
[1] เปิด session          → เลือก input_dir เดียวผ่าน DirPicker (output/test folder ถูกยกเลิกแล้ว — ระบบจัดการเองใต้ .ctflow/)
       ↓                     ⚠️ ต้องรู้ path บนเซิร์ฟเวอร์ล่วงหน้า / ไม่มี upload
       ↓                     ⚠️ path จำไว้ใน localStorage ของ browser — ย้ายเครื่องแล้วหาย (แก้ใน FR-43/FR-46)
       ↓                     ⚠️ ไม่มีที่ไหนตอบได้ว่าในระบบมีงานอะไร ใครทำ (แก้ใน FR-44)
[2] วาดกล่อง + Save       → สร้าง SAVPE embedding เข้า bank
       ↓                     ⚠️ อีกคนที่เปิดโฟลเดอร์เดียวกันได้ภาพเดียวกัน แล้วทับกันเงียบ ๆ (แก้ใน FR-48/FR-49)
       ↓                     ⚠️ ต้องพิมพ์ชื่อคลาสเองทุกครั้ง? (ต้องยืนยันจาก UI จริง)
[3] Rescore               → ดู confidence ภาพที่เหลือ เรียงต่ำสุดก่อน
       ↓                     ⚠️ เรียงตาม conf อย่างเดียว → เจอภาพซ้ำแนวเดิมติดกัน
[4] เตรียม test set       → import ภาพ + วาด ground truth
       ↓                     ⚠️ งานหนักที่ผู้ใช้อาจข้าม เพราะยังไม่เห็นประโยชน์ทันที
[5] Evaluate              → ดู precision/recall/F1
       ↓                     ⚠️ ได้ตัวเลขแต่ไม่มีคำแนะนำว่าควรทำอะไรต่อ
[6] Auto-label remaining  → ระบบเขียน label ที่เหลือ
       ↓                     ⚠️ ไม่มีตัวช่วยตัดสินใจว่า F1 เท่าไหร่ถึง "พอ"
[7] Review mode           → แก้กล่องที่ผิด
                             ⚠️ แก้แล้วไม่ช่วยสอนโมเดล แต่ UI ไม่บอก
```

### จุด friction ที่ทำให้ "น่าเบื่อ" เรียงตามผลกระทบ

| # | Friction | ทำไมทำให้เบื่อ | หลักการที่ถูกละเมิด |
|---|---|---|---|
| F1 | ไม่รู้ว่าต้อง label อีกกี่ภาพ | ไม่เห็นเส้นชัย = แรงจูงใจหาย | P3 |
| F2 | คลาสที่ตันแล้ว (`defect`) ยังถูกให้ label ต่อ | รู้สึกว่าทำไปก็ไร้ผล | P3, P5 |
| F3 | เรียงภาพตาม conf อย่างเดียว → เจอภาพคล้ายกันซ้ำๆ | ทำงานซ้ำซากโดยได้ผลน้อย | P1 |
| F4 | แก้ auto-label แล้วโมเดลไม่ดีขึ้น แต่ไม่มีใครบอก | ทำงานเสียเปล่าโดยไม่รู้ตัว | P5 |
| F5 | ไม่มี keyboard shortcut สำหรับงานซ้ำ (วาด/เลือกคลาส/ถัดไป) | ต้องใช้เมาส์ทุกขั้น ช้าและล้า | P4 |
| F6 | ไม่เห็นกราฟความคืบหน้าของ F1 ตามจำนวน prompt | ไม่รู้ว่าที่ทำมามีผลแค่ไหน | P2 |
| F7 | ต้องเตรียม test set เองก่อนถึงจะ Evaluate ได้ | งานหนักที่ยังไม่เห็นผลตอบแทนทันที | P2 |

---

## 5. Functional Requirements

สัญลักษณ์สถานะ: ✅ ทำแล้ว · 🟡 ทำบางส่วน · ❌ ยังไม่ทำ

### 5.1 กลุ่ม: Core labeling loop

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-01 | เปิด session ด้วย input_dir เดียว (output/test folder ไม่ต้องเลือกแยก, จัดการเองใต้ `.ctflow/`) | ✅ | `POST /api/session` |
| FR-02 | วาด bounding box แล้วบันทึกเป็น SAVPE embedding เข้า bank | ✅ | `POST /api/label` |
| FR-03 | บันทึก label เป็น YOLO txt format | ✅ | `labels/<stem>.txt` |
| FR-04 | class index เสถียร ไม่เปลี่ยนเมื่อเพิ่มคลาสใหม่ | ✅ | append-only + smoke test ยืนยัน |
| FR-05 | Rescore ภาพในพูลเทียบ bank ปัจจุบัน | ✅ | `POST /api/score` (background job) |
| FR-06 | Auto-label ภาพที่เหลือจาก bank | ✅ | `POST /api/autolabel` |
| FR-07 | Review/แก้ไข auto-label โดยไม่สร้าง embedding ใหม่ | ✅ | `POST /api/relabel` |
| FR-08 | แยกบันทึกว่าภาพไหน manual (`labeled`) vs auto (`auto`) | ✅ | `BankSummary` |
| FR-09 | `/api/relabel` รองรับ `mode="update"` (merge) | ✅ | ต่อจาก checkbox "Add to existing" ในหน้า review |

### 5.2 กลุ่ม: การวัดผลและตัดสินใจ

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-10 | Test set แยกขาดจาก prompt bank (ภาพในพูลที่ถูกแปะป้าย ไม่คัดลอกไฟล์) | ✅ | ไม่มี `_bank/` ใน `.ctflow/testset/` + `POST /api/label` ปฏิเสธภาพที่แปะป้ายไว้ + assertion |
| FR-11 | วัด precision/recall/F1 ที่ IoU 0.5 | ✅ | `POST /api/evaluate` |
| FR-12 | แสดง metric แยกรายคลาส | ✅ | `metrics.evaluate()` คืนทั้งรวมและต่อคลาส |
| FR-13 | **แสดงกราฟ F1 ตามจำนวน prompt (learning curve)** | ✅ | แท็บ Progress · history เก็บที่ `<input_dir>/.ctflow/_bank/eval_history.json` (`GET/POST/DELETE /api/history`) |
| FR-14 | **แจ้งเตือนเมื่อคลาสใดเข้าสู่ plateau (label เพิ่มแล้ว F1 ไม่ขยับ)** | ✅ | `adviseClass()` — F1 ขยับ < 2% ติดกัน 2 รอบที่ prompt เพิ่ม = "Stalled" |
| FR-15 | **แนะนำ action ถัดไปหลัง Evaluate** (เช่น "คลาส X ควร label เพิ่ม, คลาส Y พร้อม auto แล้ว") | ✅ | หัวข้อ "What to do next" + chip ต่อคลาสใน prompt bank |
| FR-16 | ทดลอง conf threshold ได้จาก UI โดยไม่ต้องแก้โค้ด | ✅ | slider ในการ์ด Readiness |

### 5.3 กลุ่ม: ลดภาระการ label (หัวใจของ "ง่ายและไม่น่าเบื่อ")

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-17 | เรียงภาพที่ควร label ถัดไป | ✅ | conf ต่ำสุดก่อน + diversity (ดู FR-18) · ปุ่ม Save/Skip เดินตามลำดับเดียวกับ Queue |
| FR-18 | **Active-learning selector ที่คำนึงถึง diversity ไม่ใช่แค่ conf** | 🟡 | ใช้ลายนิ้วมือภาพ 8×8 เทา จาก `/api/score` แล้ว greedy เลี่ยงภาพซ้ำแนวเดิม — **ยังไม่ใช่ระยะห่างของ embedding จริง** (T-09 เต็มรูปแบบ) |
| FR-19 | **Pre-annotation: แสดงกล่องที่โมเดลทำนายไว้ล่วงหน้าตอนผู้ใช้เปิดภาพเพื่อ label** | ✅ | `POST /api/predict` · กล่องร่างเส้นประ + ปุ่ม "Use them" (คีย์ A) · bank ว่าง = ไม่เรียก · ~0.4 วิ/ภาพ (เรียกแรกของ process ~8 วิ ตอน warm-up) · คัดกล่องที่ไม่ต้องการทิ้งได้ทีละกล่องก่อนกด "Use them" — ดู FR-37 |
| FR-20 | **Keyboard shortcut สำหรับงานซ้ำ** (next/prev image, เลือกคลาส 1-9, save, undo, ลบกล่อง) | ✅ | Enter/Ctrl+S save · →/← เปลี่ยนภาพ · 1-9 คลาส · A รับกล่องร่าง · C copy · Ctrl+Z undo · ? ดูทั้งหมด |
| FR-21 | **จำคลาสล่าสุดที่ใช้ ตั้งเป็น default ของกล่องถัดไป** | ✅ | คลาสค้างข้ามกล่องและข้ามภาพ |
| FR-22 | **Copy กล่องจากภาพก่อนหน้า** (กรณีภาพต่อเนื่องบนสายพานที่วัตถุอยู่ตำแหน่งใกล้เคียงกัน) | ✅ | ปุ่ม "Copy last" / คีย์ C |
| FR-23 | **แสดง progress ชัดเจน: label ไปแล้วกี่ภาพ / auto ไปแล้วกี่ภาพ / เหลือกี่ภาพ** | ✅ | การ์ด Progress ถาวร + badge บนแท็บ |
| FR-24 | **Undo/redo การวาดกล่อง** | ✅ | ลึก 40 ขั้น ต่อภาพ |

### 5.4 กลุ่ม: การสื่อสารกับผู้ใช้ (ป้องกันงานเสียเปล่า)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-25 | **แจ้งใน review mode ว่าการแก้นี้ไม่สอนโมเดล + ปุ่ม "สอนโมเดลด้วย" (ส่งเข้า `/api/label` แทน)** | ✅ | แถบเตือน + ปุ่มคู่ "Save corrections" / "Fix and teach" |
| FR-26 | **อธิบายศัพท์เทคนิคใน UI** (tooltip: bank, embedding, rescore) | ✅ | `Term`/`HelpDot` ครอบศัพท์ทุกคำบนหน้าจอ |
| FR-27 | **เตือนเมื่อผู้ใช้กด auto-label ทั้งที่ F1 ยังต่ำ** | ✅ | dialog แสดง F1 จริง + คำแนะนำรายคลาส ก่อนยอมให้กดต่อ (เกณฑ์ `READY_F1 = 0.75`) |
| FR-28 | แสดงเหตุผลเมื่อ auto-label ไม่เขียนป้ายให้บางภาพ (`no_detection`) | ✅ | คืน `no_detection_images` + การ์ดพร้อมปุ่มเปิดภาพแรก |

### 5.5 กลุ่ม: การเข้าถึงสำหรับผู้ใช้ที่ไม่ใช่วิศวกร (Persona B)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-29 | **อัปโหลดภาพผ่าน UI (drag & drop)** | 🟡 | **backend เสร็จแล้ว** — `POST /api/upload` (multipart) ตรวจนามสกุล + decode จริง + ขนาด + ไม่เขียนทับ + ตัด path ออกจากชื่อไฟล์ · **เหลือ dropzone บน UI** — เลื่อนไป Phase 4 เพราะยังไม่มีคำตอบว่าไฟล์ที่อัปโหลดไปลงโฟลเดอร์ไหน ใครตั้งชื่อ และลบโปรเจกต์แล้วไฟล์หายไหม |
| FR-30 | **Authentication** | ✅ | OIDC (authorization-code + PKCE + RP-initiated logout) พร้อมหน้า login/callback/logout บน UI · `LABEL_TOOL_USERS` (PBKDF2 + signed cookie) เหลือไว้เป็นทางเข้าของ CI/dev · **บังคับตั้งแต่ Phase 2 (FR-47)** — ไม่ตั้งอย่างใดอย่างหนึ่ง = แอปไม่ start |
| FR-31 | **บันทึกว่าใคร label instance ไหน** | ✅ | `labeled_by` ในทุก instance ของ `metadata.json` และ `annotations.created_by` ในทุกกล่อง — เก็บ `sub` ของ provider แล้วแปลกลับเป็นชื่อผ่านตาราง `users` |
| FR-32 | โหมดง่าย (simple mode) ที่ซ่อนศัพท์เทคนิค | ✅ | สวิตช์ "Plain language" บน top bar |

### 5.6 กลุ่ม: คุณภาพโมเดล (ปัญหาเทคนิคที่กระทบ user โดยตรง)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-33 | **แก้ปัญหาคลาสขนาดเล็ก (`defect` F1 ~0.04–0.07)** | 🟡 | **T-01 เสร็จแล้ว → [EXPERIMENT_T01_CONF.md](./history/EXPERIMENT_T01_CONF.md)** — สาเหตุคือ threshold: ที่ conf 0.25 `defect` recall = 0.00, ที่ 0.05 = 0.26 (F1 0.248) · เพิ่ม `conf_by_class` ให้ `/api/predict`, `/api/evaluate`, `/api/autolabel` แล้ว ได้ `defect` 0.248 + `good_part` 0.818 พร้อมกัน · **ยังห่างจาก `READY_F1 = 0.75` → T-08 (crop) ยังต้องทำ** |
| FR-34 | NN-matching bank แทน mean-pooling | ❌ | ทำหลัง T-08 — T-01 ชี้ว่า scale เป็นสาเหตุหลัก (defect median = 2.09% ของภาพ vs good_part 43.45%) |
| FR-35 | Retrain closed-set detector จาก label ที่สะสม | ❌ | เป้าหมายระยะยาว ยังไม่ยืนยันว่าคุ้ม |
| FR-36 | **เลือก YOLOE checkpoint ได้เอง** (หลายเวอร์ชัน/หลายขนาด แทนที่จะ hardcode ตัวเดียว) | ✅ | `backend/models.json` + `inference/models.py` — catalog ตั้งแต่ `yoloe-v8s-seg` เล็กสุด ถึง `yoloe-26x-seg` (รุ่น 26 ใหม่ล่าสุด ใหญ่สุด แม่นสุด) รวม 11 ตัวเลือก · `GET /api/config` ส่งรายการให้ dropdown · เลือก/เปลี่ยนได้ **ทุกที่ที่ตัวเลือกโมเดลปรากฏ** (Setup card ก่อนเปิด session และการ์ด "Model" ในหน้า label เอง — ทั้งคู่ใช้ `ModelPicker.tsx` ตัวเดียวกัน) ตราบใดที่โปรเจกต์ยังไม่มี embedding — `Bank.lock_model()` ล็อกโมเดลไว้กับ bank ตั้งแต่กล่องแรกที่บันทึก เพราะ embedding จากคนละโมเดลผสมกันไม่ได้ (ดู [ARCHITECTURE.md](./ARCHITECTURE.md#การเลือกโมเดล)) · ค่า default (`yoloe-11s-seg`) เหมือนพฤติกรรมเดิมทุกประการ · **บั๊กที่เคยเกิดจริงแล้วแก้แล้ว (สองรอบ):** (1) โปรเจกต์เก่าที่สอนไว้ก่อน FR-36 มี `_bank/metadata.json` ที่ไม่มี key `"model"` เลย ทำให้ `bank.model` อ่านได้ `None` แล้วส่งตรงเข้า `arm()`/`checkpoint_path(None)` จน predict/score/evaluate/autolabel ทั้งหมด `500 ValueError: unknown model None` ไปเงียบ ๆ — preview ไม่ขึ้น, eval ไม่มีผล แก้ด้วย `Bank._migrate_missing_model()` ที่รันอัตโนมัติใน `__init__`: bank ที่มี embedding อยู่แล้วแต่ไม่มี key `"model"` จะถูกล็อกเข้ากับ `DEFAULT_MODEL_ID` ทันทีและ **เขียนกลับลงดิสก์จริง** (ไม่ใช่แค่ fallback ตอน infer เฉย ๆ อย่างที่แก้รอบแรก ซึ่งพลาดเพราะ `bank.summary()["model"]` ที่ frontend ใช้ตัดสินใจว่าจะโชว์ dropdown แบบแก้ไขได้หรือ chip ล็อก ยังอ่านได้ `None` เหมือนเดิม ผู้ใช้เลยเปลี่ยนโมเดลใน dropdown ได้แต่ไม่มีผลอะไรเลยเพราะ evaluate/predict ไม่เคยรับ model_id จาก client) (2) migration ที่เพิ่มเข้ามารอบแรกไปชน bug คนละตัว: `_bank/` ของโปรเจกต์เก่าเป็นไฟล์ที่เขียนไว้ตอน container ยังรันเป็น root (ก่อนงาน non-root ของ session ก่อนหน้า) พอ container เปลี่ยนมารันเป็น `app` (non-root) ไฟล์เหล่านั้นเขียนไม่ได้อีกต่อไป (`PermissionError` ตอนขอ `.lock`) — ทั้ง `/api/session`, `/api/label` (ผ่าน `lock_model()`), autolabel ล้วนพังเงียบ ๆ ด้วยสาเหตุนี้ (ไม่ใช่แค่โปรเจกต์เดียว พบว่าทั้ง `/data` มีไฟล์ root-owned ค้างอยู่ ~4,377 รายการ) แก้ด้วย `chown -R app:app /data` หนึ่งครั้ง (รันเป็น root ผ่าน `docker compose exec -u root`) และห่อ `_migrate_missing_model()` ด้วย `except OSError: pass` ไม่ให้ migration ที่ล้มเหลวจากปัญหาสิทธิ์ไปทำให้ endpoint อ่านอย่างเดียว (เช่น session/predict) พังไปด้วย ยืนยันแล้วด้วย regression check ใน `tests/smoke_test.py` ที่ลบ key `"model"` ออกจาก metadata ตรง ๆ เพื่อจำลอง bank เก่า และทดสอบจริงกับโปรเจกต์ `iron_ore` ผ่าน container ที่รันอยู่จริง (predict/evaluate/label ทำงานได้หมดหลังแก้) |
| FR-37 | **ลบกล่องที่โมเดลทำนายผิด (over-prediction) ทีละกล่องได้ ก่อนกด "Use them"** | ✅ | `BoxCanvas.tsx` — คลิกกล่องเส้นประที่ต้องการตัดทิ้งเพื่อเลือก แล้วกดปุ่มกากบาทที่มุมกล่อง (`onRemoveDraft`) เพื่อลบออกจากกล่องที่จะรับ · เลือกได้ทีละกล่อง มี highlight บอกว่ากำลังเลือกอันไหนอยู่ · ไม่กระทบกล่องที่ยืนยันแล้ว (`onRemove` เดิมของ FR-24) เพราะ draft กับกล่องที่ยืนยันแล้วอยู่คนละ array คนละ selection state · เมาส์ชี้ค้างบนกล่องใด ๆ (ยืนยันแล้ว/draft/มุม resize) จะเปลี่ยน cursor (`pointer`/`grab`/`nwse-resize`/`nesw-resize`) แยกจากโหมดวาดกล่องใหม่ (`crosshair`) ให้เห็นก่อนคลิกว่ากำลังจะ "เลือก" ไม่ใช่ "วาด" |
| FR-38 | **บอกว่า weight ของแต่ละโมเดลพร้อมใช้จริงหรือยัง** (เลือกโมเดลที่ยังไม่มี weight บนเครื่อง = predict ครั้งแรกอาจช้ามากหรือเงียบล้มเหลวถ้าเน็ตไปโหลดจาก GitHub ไม่ได้) | ✅ | `inference/models.py` / `internal/platform/models` เช็คว่าไฟล์ `.pt` อยู่ใน `MODELS_DIR` จริงหรือไม่ ต่อโมเดล → ส่งเป็น `available: bool` ใน `GET /api/config` · UI แสดงจุดกลม 🟢/🔴 หน้าตัวเลือกแต่ละอันใน dropdown และหน้าโมเดลที่เลือกอยู่ (`ModelPicker.tsx`) · pre-cache แล้วสามตัว: `yoloe-11s-seg` (default), `yoloe-26s-seg`, `yoloe-26x-seg` — ใน `label_tool/models/` ที่เหลือยัง auto-download ตอนใช้ครั้งแรกตามปกติ (FR-36) |
| FR-39 | **เปลี่ยนโมเดลของโปรเจกต์ที่ล็อกไปแล้วได้จริง โดยไม่ต้องเริ่ม output folder ใหม่ (re-embed)** | ✅ | `Bank.reembed(model_id, new_embeddings)` (`bank.py`) — commit แบบ atomic ภายใต้ lock เดียวกับ `lock_model()`: แทนที่ embedding ทุกตัวพร้อมกัน + สลับ `bank.model` ในจังหวะเดียว ไม่มีสถานะครึ่ง ๆ กลาง ๆ ที่ concurrent read จะเห็น · `POST /api/reembed` (background job แบบเดียวกับ evaluate/autolabel, poll ผ่าน `/api/jobs/{id}`) วนอ่าน `bank.instances` ทุก class ทุก instance กลับไปที่ `source_image`+`bbox` เดิม รัน `extract_embedding()` ใหม่ด้วยโมเดลเป้าหมาย แล้วค่อย commit ทีเดียวตอนจบ · UI: ปุ่ม "Switch model…" ใต้ chip ที่ล็อกอยู่ใน `ModelPicker.tsx` เปิด dropdown เลือกโมเดลใหม่ + ต้องผ่าน `Confirm` dialog (บอกจำนวน instance ที่จะประมวลผลใหม่) ก่อนเริ่มจริง · **ไฟล์ label (`labels/*.txt`, `classes.txt`) และ `instances` (provenance) ไม่ถูกแตะเลย** — ยืนยันด้วย md5sum ของไฟล์ label ก่อน/หลังตรงกันทุกตัวอักษรในการทดสอบจริงกับ `iron_ore` (สลับ `yoloe-11s-seg` → `yoloe-26s-seg` → กลับมา `yoloe-11s-seg` ครบรอบ) · หลัง reembed สำเร็จ frontend ล้าง `scores`/`evalResult`/drafts ที่ยังค้างของโมเดลเก่าทิ้งอัตโนมัติ (`session.ts::reembedModel`) เพราะเป็นตัวเลขที่วัดด้วยโมเดลเก่าไปแล้ว **ข้อจำกัดที่รู้: instance ที่สอนจากหลายกล่องคลาสเดียวกันในการ save ครั้งเดียว** (`by_class` ใน `save_label()`) จะถูกเก็บ bbox ตัวแทนไว้แค่กล่องแรก — reembed เลย replay ได้แค่กล่องนั้นกล่องเดียว ไม่ใช่ค่าเฉลี่ยเดิมทั้งชุด (มี `ponytail:` comment กำกับไว้ใน `jobs.py::_run_reembed`) — เกิดขึ้นเฉพาะกรณี label หลายกล่องคลาสเดียวกันในภาพเดียวก่อน save ซึ่งไม่ใช่ flow หลัก |

### 5.7 กลุ่ม: Multi-user & DB-backed storage (ยังไม่ทำ — ตกลง scope กับทีมแล้ว 2026-08-21)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-40 | ย้าย label/box metadata (`labels/*.txt`, `classes.txt`, `testset.json`, สถานะ `labeled`/`auto`) จากไฟล์ไปตาราง PostgreSQL | ✅ | `internal/infra/store` + `backend/db/schema.sql` · implement แล้ว 2026-08-21 และทดสอบผ่าน PostgreSQL จริง · `_bank/embeddings.pt` ไม่อยู่ใน scope นี้ ยังเป็นไฟล์เหมือนเดิมตามแผน |
| FR-41 | รองรับหลายคนแก้ project (`input_dir`) เดียวกันพร้อมกันจริง (ไม่ใช่แค่กันเขียนชนกันแบบ `filelock`) | ✅ | DB transaction ล็อกระดับแถวตอนสร้างคลาสใหม่ใน `internal/infra/store` และมี concurrency test กับ PostgreSQL จริง |
| FR-42 | Export annotation เลือก format ได้ (YOLO/COCO/Pascal VOC) แทนที่จะได้แค่ YOLO txt | ✅ | `GET /api/export` (`internal/transport/httpapi` + `internal/core/export`) — ทดสอบผ่านทั้งสาม format จริง · **ยังไม่มี UI ให้เลือก format** |

**แรงจูงใจ:** ทีมมีแผนทำระบบ login + workspace แบบ Label Studio ในอนาคต และทีม infra ต้องการวาง PostgreSQL เป็นรากฐาน — งานกลุ่มนี้เตรียมทางไว้ (เช่น `annotations.created_by`, `projects.id` ที่ future user/workspace table จะอ้างอิง) แต่**ไม่ได้สร้างระบบ login/workspace จริงในรอบนี้** (ทำจริงใน 5.8)

### 5.8 กลุ่ม: Workspace & Multi-user (Phase 2 — ตกลง scope กับเจ้าของโปรเจกต์แล้ว 2026-08-28)

แผนเต็มและเหตุผลของทุกข้อ: [PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-43 | สร้าง / ดู / เปลี่ยนชื่อ / ลบ โปรเจกต์ได้จาก UI โดยระบุชื่อ, โฟลเดอร์ dataset และชนิดงาน | 🟡 | API ครบแล้ว (T-26): `POST/GET/PATCH/DELETE /api/projects` · ลบ = ลบแถวใน DB เท่านั้น **ไม่แตะไฟล์บนดิสก์** · UI ยังไม่มี (T-29) |
| FR-44 | Home page แสดงรายการโปรเจกต์ทั้งหมด พร้อมเจ้าของ ความคืบหน้า และคนที่ลงมือทำ | 🟡 | `GET /api/projects` คืนครบแล้วทั้งเจ้าของ/`labeled`/`auto`/`contributors` (T-26) · หน้า home ยังไม่ได้เขียน (T-29) · ไม่ซ่อนโปรเจกต์ของใครจากใคร · `labeled`/`auto` มาจาก SQL count ไม่มีการอ่านโฟลเดอร์ |
| FR-45 | รองรับชนิดงาน label แบบอื่นในอนาคตโดยไม่ต้องรื้อของเดิม | 🟡 | คอลัมน์ `task_type` มีแล้วและปฏิเสธค่าอื่นนอกจาก `detection` ที่ handler (T-26) · โครง routing ยังไม่มี · **ไม่มี abstraction** โดยตั้งใจ |
| FR-46 | แต่ละโปรเจกต์มี URL ของตัวเองที่แชร์กันได้ | 🟡 | `GET /api/projects/{id}` พร้อมแล้ว (แปลง id → `input_dir`) · route `/p/{project_id}` ฝั่ง frontend ยังไม่มี (T-30) — `id` เป็นกุญแจสำหรับอ้างถึง ส่วน `input_dir` ยังเป็นกุญแจสำหรับเก็บ |
| FR-47 | บังคับ login — ไม่มี OIDC และไม่มี `LABEL_TOOL_USERS` = แอปไม่ start | ✅ | T-27 · `cmd/api/main.go` exit non-zero พร้อมข้อความบอกทางแก้ · `RequireLogin` fail closed เองด้วย ไม่พึ่ง boot check อย่างเดียว · ลบ `LABEL_TOOL_MODE=local` ทิ้งแล้วทั้ง Go และ sidecar `PathAllowed` เหลือสาขาเดียว |
| FR-48 | เห็นความคืบหน้าของคนอื่นในโปรเจกต์เดียวกันโดยไม่ต้อง refresh | ❌ | `GET /api/state` poll ทุก 15 วินาที · หยุด poll เมื่อ tab ไม่ active · ไม่ใช้ websocket |
| FR-49 | สองคน label โปรเจกต์เดียวกันแล้วไม่ถูกเสนอภาพเดียวกัน | ❌ | จองภาพใน memory TTL 10 นาที · **เป็นคำแนะนำ ไม่ใช่ล็อก** — `POST /api/label` ไม่เคยปฏิเสธเพราะเรื่องจอง |
| FR-50 | ดูได้ว่าใคร label กล่องไหน และใครทำงานในโปรเจกต์ไหนบ้าง | 🟡 | `contributors` ใน `GET /api/projects` derive จาก `annotations.created_by` join `users` แล้ว (T-26) — เก็บ *ความจริง* ไม่ใช่ *ความตั้งใจ* จึงไม่มีตาราง members · UI ยังไม่แสดง (T-34) · ⚠️ **ข้อค้าง:** วันนี้ response ส่ง `oid` มาด้วยและ fallback ใช้ `oid` เป็น `username` เมื่อไม่มีแถวใน `users` (local login ทุกคน) ซึ่งขัดกับ "ไม่ส่ง `oid` ดิบไปให้ UI แสดง" — ต้องตัดสินก่อนทำ T-34 |
| FR-51 | เตือนเมื่อ prompt bank กับ PostgreSQL ไม่ตรงกัน | ❌ | `POST /api/session` คืน `bank_orphaned` — bank มี embedding แต่ DB ไม่มีแถว `images` เลย · เกิดได้สองทาง: `docker compose down -v` โดยไม่ลบ `.ctflow/` **และตั้งแต่ T-26 คือ `DELETE /api/projects/{id}` แล้วเปิดโฟลเดอร์เดิมอีกครั้ง** (ดู comment ที่ `httpapi/projects.go::DeleteProject`) |

**สิ่งที่ Phase 2 ตั้งใจไม่ทำ:** role/permission · ตาราง project members · upload UI · abstraction ต่อ task type · migration framework · deep link ถึงภาพรายตัว · real-time แบบ websocket

---

## 6. Non-Functional Requirements

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| NFR-01 | Path safety — กัน browser เข้าถึงไฟล์นอกขอบเขต | ✅ | `internal/platform/config.PathAllowed` + `vm` mode |
| NFR-02 | กันการเขียน bank ชนกัน | ✅ | `filelock.FileLock` |
| NFR-03 | Background job ไม่ block UI | ✅ | Go goroutine + `/api/jobs/{id}` polling ทุก 400ms |
| NFR-04 | ETA คำนวณจากเวลาเซิร์ฟเวอร์ (กัน clock skew) | ✅ | `started_at`/`now` |
| NFR-05 | CI รัน smoke test อัตโนมัติ | ✅ | `.github/workflows/backend.yml` — jobs `go`, `python` และ `smoke` · smoke รัน Go API + Python sidecar จริงผ่าน HTTP · ยืนยันแล้วว่าจับ regression ของ class order, IoU threshold และ path safety ได้ · merge commit `40b0c7f` ผ่านบน GitHub แล้ว |
| NFR-06 | Job tracker persist/รองรับหลาย worker | ❌ | ต้องใช้ Redis/TTL (มีคอมเมนต์ในโค้ดแล้ว) · เงื่อนไขเริ่มงานคือ >1 worker ซึ่งยังไม่เกิด |
| NFR-07 | Container ไม่รันเป็น root | ✅ | Dockerfile สร้าง user `app` จาก build arg `APP_UID` (default 1000) แล้ว `USER app` · บน Linux host ต้อง `--build-arg APP_UID=$(id -u)` ให้ตรงกับเจ้าของ `DATA_DIR` และต้องตรวจ ownership ของข้อมูลเก่าก่อน deploy |
| NFR-08 | HTTPS | ❌ | ต้องพึ่ง reverse proxy ภายนอก |
| NFR-09 | GPU support | 🟡 | `backend/inference/Dockerfile` ติดตั้ง PyTorch จาก `--extra-index-url .../whl/cu126` เป็นค่าเริ่มต้น (ตรงกับ driver 12.7 ที่ตรวจจริงบนเครื่องนี้ผ่าน `nvidia-smi` — driver รองรับ CUDA ≥ 12.6) + `docker-compose.yml` ขอ GPU ผ่าน `deploy.resources.reservations.devices` · override เป็น CPU ได้ด้วย `--build-arg TORCH_INDEX_URL=.../whl/cpu` โดยไม่ต้องแก้ไฟล์ · `device: None` ใน `inference/vpe.py` ปล่อยให้ ultralytics auto-เลือก cuda:0 เอง ไม่ต้องแก้โค้ด inference · **`docker compose build api` สำเร็จจริง** (ยืนยันแล้วว่า `torch-2.13.0+cu126` ติดตั้งได้ไม่มี error) **แต่ยังไม่ได้ยืนยัน runtime** — Docker Desktop daemon ตอบ `500` ทุก request หลัง build เสร็จ (`docker info`/`docker images` ค้าง) ยังไม่ทันได้ `docker compose up` มาเช็ค `torch.cuda.is_available()` จริงในคอนเทนเนอร์ที่รัน |

---

## 7. Success Metrics

เพื่อตอบคำถาม "ง่ายและไม่น่าเบื่อจริงหรือยัง" ด้วยข้อมูล ไม่ใช่ความรู้สึก **ระบบต้องเก็บ metric เหล่านี้เอง**

ที่เก็บคือ `<input_dir>/.ctflow/_bank/events.jsonl` (append-only, หนึ่ง event ต่อบรรทัด) อ่านสรุปผ่าน `GET /api/events` — ดู `internal/infra/events`

| Metric | นิยาม | เป้าหมายที่แนะนำ | สถานะการเก็บ |
|---|---|---|---|
| **Time-to-first-auto** | เวลาตั้งแต่เปิด session จนกด auto-label ครั้งแรกได้ | < 30 นาที ต่อคลาสที่ไม่ยาก | 🟡 backend เก็บและสรุปได้แล้ว (`median_time_to_first_auto_secs`) · UI ยังอ่านจาก state ของ session ปัจจุบัน |
| **Prompts-to-plateau** | จำนวน prompt ที่ต้อง label ก่อน F1 หยุดเพิ่ม | ยิ่งน้อยยิ่งดี — ใช้เทียบระหว่างคลาส | ✅ อ่านจาก `eval_history.json` (แกน X ของ learning curve) |
| **Manual-label ratio** | จำนวนภาพที่ label มือ ÷ ภาพทั้งหมดในพูล | < 10% | ✅ การ์ด "Effort saved" |
| **Auto-label correction rate** | % ของภาพ auto ที่ต้องแก้ใน review mode | < 20% | 🟡 backend มี `correction_rate` แล้ว · UI ยังนับต่อ session |
| **Median time per manual label** | เวลาเฉลี่ยต่อการ label 1 ภาพด้วยมือ | ควรลดลงหลังทำ FR-19/FR-20 | 🟡 backend มี `median_label_secs` แล้ว · UI ยังนับต่อ session |
| **Session abandonment** | % session ที่เปิดแล้วไม่เคยกด auto-label | < 20% | 🟡 backend มี `abandonment` แล้ว · ยังไม่มีที่แสดงบน UI |

> **สิ่งที่เหลือคือด้าน frontend ล้วน ๆ:** ยิง `POST /api/events` ตอนเปิด session / save ป้าย / แก้ใน review / กด auto-label แล้วให้การ์ด "Effort saved" อ่านจาก `GET /api/events` แทน state ภายใน จากนั้นตัวเลขทั้งตารางจะข้าม session และข้ามเครื่องได้ทันที

> **หมายเหตุ:** ตัวเลขเป้าหมายข้างต้นเป็นข้อเสนอตั้งต้นสำหรับให้ทีมถกเถียง ยังไม่ได้ผ่านการยืนยันจากข้อมูลการใช้งานจริง — ควรเก็บ baseline ก่อน 1-2 สัปดาห์แล้วค่อยตั้งเป้าจริง

---

## 8. สมมติฐานที่ต้องตรวจสอบ

| # | สมมติฐาน | วิธีตรวจสอบ | ผลกระทบถ้าผิด |
|---|---|---|---|
| A1 | ผู้ใช้เป้าหมายระยะกลางรวมถึง QC Operator ไม่ใช่แค่ ML Engineer | ถามทีม/หัวหน้าโดยตรง | งาน upload UI (FR-29) และ simple mode ไม่จำเป็น |
| A2 | ปัญหา `defect` เกิดจาก resolution/scale ไม่ใช่ diversity | T-01 (เสร็จ) + T-08 | ลำดับ T-08 ก่อน T-11 ผิด ต้องสลับ |
| A3 | Pre-annotation (FR-19) ลดเวลา label จริง | วัด median time per label ก่อน/หลัง | เสีย effort ไปกับฟีเจอร์ที่ไม่ช่วย |
| A4 | ผู้ใช้ยอมเสียเวลาเตรียม test set ถ้าเห็นประโยชน์ชัด | สังเกตว่ามีกี่ session ที่ข้ามขั้น evaluate | ต้องออกแบบวิธีวัดผลที่ไม่ต้องพึ่ง test set |
| A5 | ภาพบนสายพานต่อเนื่องพอที่ copy กล่องจากภาพก่อนหน้า (FR-22) จะช่วยได้ | ดูตัวอย่างภาพจริงจาก dataset | FR-22 ไม่คุ้มทำ |
| A6 | ~~จำนวนผู้ใช้พร้อมกันยังน้อย (1 คนต่อโปรเจกต์)~~ | — | **ตัดสินใจแล้ว 2026-08-21: ผิด** — ต้องรองรับหลายคนพร้อมกันจริง → label storage ย้ายไป PostgreSQL |
| A7 | ~~คนทำงานพร้อมกันต่อโปรเจกต์มีมากพอที่จะต้องมีระบบ assignment~~ | — | **ตัดสินใจแล้ว 2026-08-28: ผิด** — จริง ๆ มี 1–2 คนต่อโปรเจกต์ → ใช้ self-serve queue + จองภาพ ไม่ทำ assignment ([PHASE2_WORKSPACE.md](./PHASE2_WORKSPACE.md) ข้อ 2) |
| A8 | ~~ต้องรองรับการรันบนเครื่องส่วนตัว (`LABEL_TOOL_MODE=local`) ต่อไป~~ | — | **ตัดสินใจแล้ว 2026-08-28: ผิด** — ไม่มีใครใช้แล้ว → ลบ `local` mode และบังคับ login (FR-47) |
| A9 | จะมีโมดูล label แบบอื่นนอกจาก detection จริง (เช่น LPR) | มีงานจริงเข้ามาหรือยัง | `task_type` + การผ่าโฟลเดอร์โมดูลเป็นการลงทุนที่เปล่าประโยชน์ — **แต่ราคาถูกมากจนยอมเสี่ยงได้** (1 คอลัมน์ + การย้ายไฟล์) |

### คำถามที่ยังไม่มีคำตอบ

1. **จะรองรับ QC Operator จริงหรือให้ ML Engineer เป็นตัวกลางต่อไป** — ตัดสินใจก่อนลงทุน upload UI (FR-29)
2. **ไฟล์ที่อัปโหลดผ่าน UI ไปลงโฟลเดอร์ไหน ใครตั้งชื่อ ลบโปรเจกต์แล้วไฟล์หายไหม** — ข้อนี้คือเหตุผลที่ FR-29 ถูกเลื่อนออกจาก Phase 2
3. **เกณฑ์ F1 เท่าไหร่ถึงถือว่า "พอ" สำหรับ auto-label** — วันนี้ใช้ `READY_F1 = 0.75` เป็นค่าที่ตั้งเอง ยังไม่มีใครยืนยัน

---

## ภาคผนวก — สรุปสถานะแบบย่อ

**ทำเสร็จแล้ว (พร้อมใช้งาน):** core labeling loop ครบวงจร (label → bank → rescore → evaluate → auto-label → review), label/box storage บน PostgreSQL, class-index stability, test set แยกขาดจาก bank, export 3 format, background job pattern, path safety, เลือก/สลับ YOLOE checkpoint ได้ (FR-36/FR-39), OIDC login ครบทั้ง backend และ UI

**ช่องว่างที่กระทบผู้ใช้มากที่สุดตอนนี้:**
1. **ไม่มีแนวคิด "โปรเจกต์" ในสายตาผู้ใช้** — path ถูกจำใน `localStorage` ของ browser, ไม่มีที่ไหนตอบได้ว่าในระบบมีงานอะไรใครทำ → **Phase 2 (FR-43…FR-51)**
2. **สองคนหยิบภาพเดียวกัน** — คิวเรียงเหมือนกันทุกคนและไม่มี refresh → **FR-48/FR-49**
3. **คลาส `defect` ยังไม่ถึงเกณฑ์ auto-label** — รู้สาเหตุแล้ว (threshold + scale) และ `conf_by_class` ดึงจาก F1 0.00 → 0.248 แล้ว ที่เหลือต้องพึ่ง crop-before-SAVPE (T-08, เลื่อนไปหลัง Phase 2)
4. **ไม่มี upload UI** — backend เสร็จ แต่ยังไม่มีคำตอบว่าไฟล์ไปลงที่ไหน (FR-29)

**ช่องว่างเชิง infra:** job tracker + image claims รองรับ process เดียว (NFR-06) · ไม่มี HTTPS ในตัวแอป (NFR-08) · ไม่มี CI ฝั่ง frontend · GPU build สำเร็จแล้วแต่ runtime ยังไม่ได้ยืนยัน (NFR-09)

**ลำดับการลงมือ:** [ROADMAP.md](./ROADMAP.md)
