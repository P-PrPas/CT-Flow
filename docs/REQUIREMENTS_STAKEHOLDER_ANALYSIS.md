# Label Tool — Requirement & Stakeholder Analysis

> **วัตถุประสงค์ของเอกสารนี้:** วิเคราะห์ความต้องการจากมุมผู้ใช้เป็นหลัก เพื่อตอบคำถามเดียว — *"ต้องทำอะไรบ้าง ผู้ใช้ถึงจะ label ได้ง่าย เร็ว และไม่น่าเบื่อ"* — พร้อมระบุสถานะปัจจุบัน (ทำแล้ว/ยังไม่ทำ) ในรูปแบบที่ coding agent นำไปทำงานต่อได้ทันที
>
> **เอกสารที่เกี่ยวข้อง:** [PRODUCT_OVERVIEW.md](./PRODUCT_OVERVIEW.md) · [ARCHITECTURE.md](./ARCHITECTURE.md) · [API_REFERENCE.md](./API_REFERENCE.md) · [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md) · [PROJECT_STATUS.md](./PROJECT_STATUS.md) · [NEXT_STEPS.md](./NEXT_STEPS.md) · [GLOSSARY.md](./GLOSSARY.md)
>
> **สถานะ:** ฉบับร่างสำหรับทบทวนกับทีม — ส่วนที่เป็น requirement ใหม่ยังไม่ผ่านการยืนยันกับผู้ใช้จริง (ดูหัวข้อ 9 "สมมติฐานที่ต้องตรวจสอบ")
>
> **หมายเหตุหลัง merge:** ลำดับงานที่ active ให้ยึด [NEXT_STEPS.md](./NEXT_STEPS.md) เป็น source of truth; phase ในเอกสารนี้เป็นการจัดกลุ่ม requirements เดิม

---

## สารบัญ

1. [หลักการชี้นำ: ทำไม "ไม่น่าเบื่อ" ถึงเป็น requirement ไม่ใช่ nice-to-have](#1-หลักการชี้นำ)
2. [Stakeholder analysis](#2-stakeholder-analysis)
3. [User personas และ pain point](#3-user-personas-และ-pain-point)
4. [User journey ปัจจุบัน + จุดที่เกิด friction](#4-user-journey-ปัจจุบัน--จุดที่เกิด-friction)
5. [Functional requirements (แยกสถานะ)](#5-functional-requirements)
6. [Non-functional requirements](#6-non-functional-requirements)
7. [Success metrics — วัดว่า "ง่ายและไม่น่าเบื่อ" สำเร็จหรือยัง](#7-success-metrics)
8. [Next steps เรียงตามลำดับสำหรับ coding agent](#8-next-steps-สำหรับ-coding-agent)
9. [สมมติฐานที่ต้องตรวจสอบก่อนลงทุนงานใหญ่](#9-สมมติฐานที่ต้องตรวจสอบ)

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

**ปัจจุบันเครื่องมือรองรับเฉพาะ ML Engineer เท่านั้น** — การไม่มีปุ่มอัปโหลดและไม่มี auth ทำให้ QC Operator (คนที่รู้จริงที่สุดว่าอะไรคือ defect) เข้ามาใช้เองไม่ได้เลย ต้องมี engineer เป็นตัวกลางเสมอ

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
[2] วาดกล่อง + Save       → สร้าง SAVPE embedding เข้า bank
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
| FR-29 | **อัปโหลดภาพผ่าน UI (drag & drop)** | 🟡 | **backend เสร็จแล้ว** — `POST /api/upload` (multipart) ตรวจนามสกุล + decode จริง + ขนาด + ไม่เขียนทับ + ตัด path ออกจากชื่อไฟล์ · ปฏิเสธด้วย 403 ถ้า `vm` mode ยังไม่มี auth · **เหลือ dropzone บน UI** |
| FR-30 | **Authentication พื้นฐาน** | 🟡 | **backend เสร็จแล้ว** — `/api/auth/{me,login,logout}` + middleware ที่ปิดทุก endpoint ยกเว้น `/api/config` · pbkdf2 + cookie เซ็น HMAC จาก stdlib · **ปิดอยู่จนกว่าจะตั้ง `LABEL_TOOL_USERS`** · เหลือหน้า login บน UI |
| FR-31 | **บันทึกว่าใคร label instance ไหน** | ✅ | `labeled_by` ในทุก instance ของ `metadata.json` (เป็น `null` เมื่อไม่ได้เปิด auth) |
| FR-32 | โหมดง่าย (simple mode) ที่ซ่อนศัพท์เทคนิค | ✅ | สวิตช์ "Plain language" บน top bar |

### 5.6 กลุ่ม: คุณภาพโมเดล (ปัญหาเทคนิคที่กระทบ user โดยตรง)

| ID | Requirement | สถานะ | หมายเหตุ |
|---|---|---|---|
| FR-33 | **แก้ปัญหาคลาสขนาดเล็ก (`defect` F1 ~0.04–0.07)** | 🟡 | **T-01 เสร็จแล้ว → [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md)** — สาเหตุคือ threshold: ที่ conf 0.25 `defect` recall = 0.00, ที่ 0.05 = 0.26 (F1 0.248) · เพิ่ม `conf_by_class` ให้ `/api/predict`, `/api/evaluate`, `/api/autolabel` แล้ว ได้ `defect` 0.248 + `good_part` 0.818 พร้อมกัน · **ยังห่างจาก `READY_F1 = 0.75` → T-08 (crop) ยังต้องทำ** |
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

**แรงจูงใจ:** ทีมมีแผนทำระบบ login + workspace แบบ Label Studio ในอนาคต และทีม infra ต้องการวาง PostgreSQL เป็นรากฐาน — งานกลุ่มนี้เตรียมทางไว้ (เช่น `annotations.created_by`, `projects.id` ที่ future user/workspace table จะอ้างอิง) แต่**ไม่ได้สร้างระบบ login/workspace จริงในรอบนี้**

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

## 8. Next Steps สำหรับ Coding Agent

จัดกลุ่มเป็น phase ตามลำดับที่ควรทำ แต่ละงานระบุ requirement ID, ไฟล์ที่เกี่ยวข้อง และเกณฑ์ยอมรับ

### Phase 0 — งานที่ควรทำก่อนทุกอย่าง (ต้นทุนต่ำ ความเสี่ยงต่ำ)

#### T-01 · ✅ ทดลอง conf threshold กับคลาส `defect` — **เสร็จแล้ว → [EXPERIMENT_T01_CONF.md](./EXPERIMENT_T01_CONF.md)**

> **ผลสรุป:** recall ขยับจริงและขยับมาก (0.00 → 0.26 เมื่อลด conf จาก 0.25 เป็น 0.05) · `defect` และ `good_part` ต้องการ threshold คนละค่า จึงเพิ่ม `conf_by_class` เข้าไปแล้ว · `defect` ยังได้แค่ F1 0.248 → **T-08 ยังต้องทำ** และเงื่อนไขเริ่มงานผ่านแล้ว

- **เชื่อมโยง:** FR-33
- **บริบท:** `/api/score` ใช้ `conf=0.05` แต่ `/api/evaluate` default `conf=0.25` — ต่างกัน 5 เท่า สัญญาณของ `defect` อาจอ่อนแต่ยังมีอยู่ใต้ threshold ปัจจุบัน
- **สิ่งที่ต้องทำ:** รัน `POST /api/evaluate` กับ dataset `conveyor_pvc` ที่ `conf` = 0.05, 0.10, 0.15, 0.25 บันทึก precision/recall/F1 ของคลาส `defect` แต่ละค่า
- **เกณฑ์ยอมรับ:** มีตารางเปรียบเทียบผลครบทุกค่า พร้อมข้อสรุปว่า recall ขยับหรือไม่
- **หมายเหตุ:** ผลลัพธ์นี้ตัดสินว่าจะลงทุนงาน T-08 (crop-before-SAVPE) หรือไม่ **ห้ามข้ามขั้นนี้ไปทำ T-08 เลย**

#### T-02 · ✅ ตั้ง CI รัน smoke test
- **เชื่อมโยง:** NFR-05
- **ทำแล้ว:** `.github/workflows/backend.yml` — jobs `go`, `python` และ `smoke`; job `go` รัน vet/gofmt/tests/build, job `python` รัน self-check ที่เหลือ และ job `smoke` โหลด weight แล้วยก Go API + sidecar จริงผ่าน HTTP
- **เกณฑ์ยอมรับ:** ✅ smoke test fail จริงเมื่อเปลี่ยน `Bank.classes` เป็น `sorted()` (ป้ายเก่าถูกอ่านเป็นคลาสผิด) · ✅ workflow รันบน GitHub แล้วและ merge commit `40b0c7f` ผ่าน

### Phase 1 — Quick win ด้าน UX (ผลตอบแทนสูงสุดต่อ effort)

#### T-03 · Keyboard shortcuts
- **เชื่อมโยง:** FR-20 (แก้ F5, P4)
- **สิ่งที่ต้องทำ:** เพิ่ม shortcut ในหน้า label — `→`/`←` เปลี่ยนภาพ, `1`-`9` เลือกคลาส, `Ctrl+S` save, `Ctrl+Z` undo, `Delete` ลบกล่องที่เลือก, `?` แสดงรายการ shortcut
- **เกณฑ์ยอมรับ:** ทำ label 1 ภาพจนจบได้โดยไม่แตะเมาส์เลย (ยกเว้นการวาดกล่อง) + มีหน้าช่วยเหลือแสดง shortcut ทั้งหมด

#### T-04 · จำคลาสล่าสุด + แสดง progress เด่นชัด
- **เชื่อมโยง:** FR-21, FR-23
- **สิ่งที่ต้องทำ:** กล่องใหม่ default เป็นคลาสที่ใช้ล่าสุดใน session; แสดงแถบ progress ถาวรบนหน้าจอ (`labeled X / auto Y / เหลือ Z จากทั้งหมด N`)
- **เกณฑ์ยอมรับ:** วาดกล่องคลาสเดิมติดกัน 5 กล่องโดยไม่ต้องเลือกคลาสซ้ำ

#### T-05 · Pre-annotation ตอนเปิดภาพเพื่อ label
- **เชื่อมโยง:** FR-19 (ผลกระทบสูงสุดในกลุ่มนี้)
- **สิ่งที่ต้องทำ:** เมื่อผู้ใช้เปิดภาพที่ยังไม่ label และ bank ไม่ว่าง ให้เรียก predict แล้วแสดงกล่องที่โมเดลทำนายไว้เป็นร่าง (สีต่างจากกล่องที่ผู้ใช้ยืนยันแล้ว) ผู้ใช้แก้/ลบ/ยืนยันแล้วค่อย Save
- **เกณฑ์ยอมรับ:** กล่องร่างแยกแยะได้ชัดจากกล่องจริงด้วยสายตา; การกด Save ส่งเฉพาะกล่องที่ผู้ใช้ยืนยัน; ถ้า bank ว่างไม่มีการเรียก predict (ไม่เสียเวลาเปล่า)
- **ข้อควรระวัง:** ต้องไม่ทำให้การเปิดภาพช้าจนน่ารำคาญ — ถ้า predict เกิน ~1 วินาที ให้แสดงภาพก่อนแล้วเติมกล่องร่างทีหลังแบบ async

#### T-06 · สื่อสารเรื่อง review mode ไม่สอนโมเดล
- **เชื่อมโยง:** FR-25 (แก้ F4, P5)
- **สิ่งที่ต้องทำ:** ในหน้า review แสดงข้อความชัดเจนว่าการแก้นี้ไม่เพิ่ม embedding เข้า bank + เพิ่มปุ่ม "สอนโมเดลด้วยภาพนี้" ที่ส่งไป `POST /api/label` แทน `/api/relabel`
- **เกณฑ์ยอมรับ:** ผู้ใช้เลือกได้ชัดเจนระหว่าง "แค่แก้ป้าย" กับ "แก้ป้าย + สอนโมเดล"; bank summary เปลี่ยนเฉพาะกรณีหลัง

### Phase 2 — ทำให้ผู้ใช้รู้ว่า "พอหรือยัง"

#### T-07 · เก็บ history ของ evaluate + แสดง learning curve
- **เชื่อมโยง:** FR-13, FR-14, FR-15 (แก้ F1, F2, F6 — หัวใจของ P2/P3)
- **สิ่งที่ต้องทำ:**
  1. เก็บผลทุกครั้งที่ evaluate ลง `<input_dir>/.ctflow/_bank/eval_history.json` — บันทึก `{timestamp, conf, จำนวน prompt ต่อคลาส ณ ขณะนั้น, metric รวม, metric ต่อคลาส}`
  2. แสดงกราฟ F1 (แกน Y) เทียบจำนวน prompt (แกน X) แยกเส้นต่อคลาส
  3. ตรวจจับ plateau: ถ้า F1 ของคลาสใดไม่เพิ่มเกิน threshold (เช่น +0.02) ตลอด N ครั้ง evaluate ล่าสุดที่ prompt เพิ่มขึ้น ให้แสดงคำเตือนว่าคลาสนี้อาจตันแล้ว
  4. แสดงคำแนะนำ action หลัง evaluate เช่น "คลาส good_part พร้อม auto-label แล้ว (F1 0.79) / คลาส defect ตันที่ F1 0.06 — พิจารณาปรับวิธีแทนการ label เพิ่ม"
- **เกณฑ์ยอมรับ:** เปิด session เดิมกลับมาแล้วกราฟยังอยู่ครบ (persist บนดิสก์); คำเตือน plateau ทำงานจริงกับข้อมูล `defect` ที่มีอยู่
- **หมายเหตุสำคัญ:** งานนี้แก้ pain point อันดับ 1 ของ Persona A โดยตรง — ผู้ใช้จะเลิกเสียเวลา label คลาสที่ตันแล้ว

#### T-08 · Crop-before-SAVPE สำหรับคลาสขนาดเล็ก (ทำต่อจากผล T-01)
- **เชื่อมโยง:** FR-33
- **เงื่อนไขเริ่มงาน:** T-01 ต้องเสร็จก่อน และผลชี้ว่าการปรับ threshold อย่างเดียวไม่พอ
- **สิ่งที่ต้องทำ:** ทดลอง pipeline สองขั้น — ตรวจ region ของชิ้นงานก่อน (คลาสที่ทำงานได้ดีอยู่แล้ว) → crop region นั้น → ส่ง crop เข้า SAVPE รอบสองเพื่อหา defect ภายใน แทนการให้ SAVPE จับทั้งสองคลาสจากภาพเต็มที่ 640px
- **เกณฑ์ยอมรับ:** มีผลเปรียบเทียบ F1 ของ `defect` ระหว่างวิธีเดิมกับวิธีใหม่บน test set ชุดเดียวกัน + รายงานผลกระทบต่อเวลา inference ต่อภาพ
- **หมายเหตุ:** สมมติฐานคือปัญหาเกิดจาก **scale/resolution** ไม่ใช่ **diversity** — ดังนั้น crop มาก่อน NN-matching (T-11)

### Phase 3 — ลดจำนวนภาพที่ต้อง label

#### T-09 · Active-learning selector ที่คำนึงถึง diversity
- **เชื่อมโยง:** FR-18 (แก้ F3, P1)
- **สิ่งที่ต้องทำ:** แทนที่การเรียง conf ต่ำสุดอย่างเดียว ด้วยคะแนนผสมระหว่าง (ก) uncertainty จาก conf และ (ข) ความต่างจาก embedding ที่มีอยู่ใน bank แล้ว — เลือกภาพที่ทั้งไม่มั่นใจ *และ* ไม่ซ้ำกับที่เคยสอนไปแล้ว
- **เกณฑ์ยอมรับ:** ในชุดทดสอบที่มีภาพซ้ำแนวเดิมจำนวนมาก ระบบต้องไม่เสนอภาพที่คล้ายกันติดกันเกิน 2 ภาพ; เทียบ prompts-to-plateau กับวิธีเดิมแล้วดีขึ้นหรือเท่าเดิม

#### T-10 · Copy กล่องจากภาพก่อนหน้า
- **เชื่อมโยง:** FR-22
- **สิ่งที่ต้องทำ:** ปุ่ม/shortcut คัดลอกกล่องทั้งหมดจากภาพที่ label ล่าสุดมาวางบนภาพปัจจุบันเพื่อแก้ต่อ (เหมาะกับภาพสายพานที่วัตถุอยู่ตำแหน่งใกล้เคียงกัน)
- **เกณฑ์ยอมรับ:** ใช้ได้เฉพาะเมื่อมีภาพก่อนหน้าที่ label แล้วใน session เดียวกัน

#### T-11 · ประเมิน NN-matching bank
- **เชื่อมโยง:** FR-34
- **เงื่อนไขเริ่มงาน:** T-08 เสร็จแล้ว และยืนยันว่า mean-pooling เป็นคอขวดจริง (ไม่ใช่ปัญหา resolution ล้วน)
- **สิ่งที่ต้องทำ:** implement NN-matching ตามคอมเมนต์ที่มีอยู่ใน `bank.py` แล้วเทียบกับ mean-pooling บน test set เดียวกัน — รายงานทั้ง F1 และเวลา inference ที่เพิ่มขึ้นตาม bank size
- **เกณฑ์ยอมรับ:** มีผลเปรียบเทียบครบทั้งสองมิติ (คุณภาพ + ความเร็ว) พร้อมข้อเสนอว่าควรใช้วิธีไหนเป็น default

### Phase 4 — ขยายไปยังผู้ใช้ที่ไม่ใช่วิศวกร

#### T-12 · 🟡 Local authentication compatibility + บันทึกผู้ label — **backend เสร็จ; OIDC เป็น milestone ถัดไป**
- **เชื่อมโยง:** FR-30, FR-31, NFR-07
- **ทำแล้ว:** `internal/platform/auth` (PBKDF2 + signed session cookie) + `internal/transport/httpapi/auth.go` และ middleware ที่ปิดทุก path ยกเว้น `/api/config` และ `/api/auth/*` · `labeled_by` ลงทุก instance
- **ปิดอยู่โดย default:** ไม่ตั้ง `LABEL_TOOL_USERS` = ไม่มี login เลย พฤติกรรมเดิมทุกอย่าง; local auth นี้จะถูกทบทวนเมื่อออกแบบ OIDC ตาม [NEXT_STEPS.md](./NEXT_STEPS.md)
- **เกณฑ์ยอมรับ:** ✅ ยืนยันใน smoke test — ไม่ login ได้ 401 ทุก endpoint, รหัสผิดได้ 401, login แล้วผ่าน, `labeled_by == "alice"`, logout แล้วกลับเป็น 401
- **ยังไม่ทำ:** หน้า login บน UI (ตอนนี้ต้อง login ผ่าน `POST /api/auth/login` เอง)

#### T-13 · 🟡 อัปโหลดภาพผ่าน UI — **backend เสร็จ**
- **เชื่อมโยง:** FR-29
- **ทำแล้ว:** `POST /api/upload` (multipart) — `checked_path()` เหมือน endpoint อื่น, ตัด directory ออกจากชื่อไฟล์, กรองนามสกุลแล้ว **decode ด้วย OpenCV จริง** (ด่านที่ตัดสิน ไม่ใช่นามสกุล), จำกัดขนาดด้วย `LABEL_TOOL_MAX_UPLOAD_MB`, ไม่เขียนทับไฟล์เดิมเด็ดขาด
- **เงื่อนไข T-12 บังคับในโค้ด:** `vm` mode + ไม่มี `LABEL_TOOL_USERS` → 403 ไม่ใช่แค่เขียนเตือนไว้ในเอกสาร
- **เกณฑ์ยอมรับ:** ✅ ยืนยันใน smoke test ครบทุกกรณีปฏิเสธ + `../escape.jpg` ลงในโฟลเดอร์ปลายทาง ไม่ใช่นอกโฟลเดอร์
- **ยังไม่ทำ:** dropzone บน UI

#### T-14 · Simple mode + tooltip อธิบายศัพท์
- **เชื่อมโยง:** FR-26, FR-32
- **สิ่งที่ต้องทำ:** โหมดที่ซ่อนคำว่า bank/embedding/rescore แล้วใช้ภาษาที่ผู้ใช้ทั่วไปเข้าใจ (เช่น "สอนตัวอย่างให้ระบบ" แทน "เพิ่ม embedding เข้า bank") + tooltip อธิบายทุกศัพท์เทคนิคที่เหลือ
- **เกณฑ์ยอมรับ:** ผู้ใช้ที่ไม่รู้จัก YOLO ทำ loop ครบตั้งแต่ label จนถึง auto-label ได้โดยไม่ต้องถามใคร (ทดสอบกับคนจริง 1 คน)

### Phase 5 — Infra / scale (ทำเมื่อมีความต้องการจริง)

| Task | Requirement | เงื่อนไข |
|---|---|---|
| T-15 · ย้าย job tracker ไป Redis + TTL | NFR-06 | เมื่อต้องรองรับหลาย worker (ยังไม่เกิด) |
| T-16 · ✅ กำหนด UID คงที่แทน root | NFR-07 | ทำแล้ว — `ARG APP_UID` + `USER app` · **build ยืนยันสำเร็จจริงในเซสชันนี้** (`docker compose build api` ผ่านทุก step รวมขั้น `useradd`/`chown`) — ยังไม่ได้ `docker compose up` เพื่อยืนยัน runtime เพราะ Docker Desktop daemon ค้างหลัง build เสร็จ (ดู NFR-09) |
| T-17 · ✅ เพิ่ม `mode="update"` ให้ `/api/relabel` | FR-09 | ทำแล้ว |
| T-18 · 🟡 GPU support | NFR-09 | `--extra-index-url .../whl/cu126` เป็นค่า build default แล้ว + GPU reservation ใน compose · **build image สำเร็จจริง** (ติดตั้ง `torch-2.13.0+cu126` + `torchvision-0.28.0+cu126` ผ่านทุก step) แต่ **ยังไม่ได้ยืนยัน runtime** (`torch.cuda.is_available()` ในคอนเทนเนอร์ที่รันจริง) — Docker Desktop daemon ค้าง (`500` บนทุก API call) หลัง build เสร็จ ไม่ทันได้ `docker compose up` |
| T-19 · Retrain closed-set detector | FR-35 | หลังพิสูจน์ว่า label สะสมพอและคุ้มกว่าใช้ prompt bank ต่อ |
| T-20 · ✅ เลือกโมเดล YOLOE ได้หลายเวอร์ชัน/ขนาด | FR-36 | ทำแล้ว — รายละเอียดที่ FR-36 · เอาการพึ่งพา `poc/yoloe-11s-seg.pt` นอก repo ออกไปพร้อมกัน (checkpoint ทุกตัว auto-download เข้า `MODELS_DIR` แทน) |

### Phase 6 — Multi-user & DB-backed annotation storage (✅ T-21–T-24 เสร็จ 2026-08-21, T-25 config เขียนแล้วยังไม่ build)

ย้าย label/box metadata จากไฟล์ YOLO txt ไปตาราง PostgreSQL เพื่อรองรับหลายคนแก้ project เดียวกันพร้อมกันจริง (เตรียมทางสำหรับ login + workspace แบบ Label Studio ในอนาคต) พร้อม export ที่เลือก format ได้ — implement และทดสอบผ่านแล้ว (`tests/smoke_test.py` เต็มรูปแบบกับ PostgreSQL จริง) รายละเอียดสิ่งที่ต่างจากแผนเดิม + สิ่งที่ยังไม่ยืนยัน (docker build image จริง, frontend UI) อยู่ที่ [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md) หัวข้อ 10

- **เชื่อมโยง:** FR-40, FR-41, FR-42
- **เหลือ:** `docker compose build api` ยังไม่ได้รันจริง (build/packaging risk เท่านั้น), export format picker บน frontend UI

---

## 9. สมมติฐานที่ต้องตรวจสอบ

เอกสารนี้ตั้งอยู่บนสมมติฐานหลายข้อที่ **ยังไม่ได้ยืนยันกับผู้ใช้จริง** ควรตรวจสอบก่อนลงทุนงานใหญ่:

| # | สมมติฐาน | วิธีตรวจสอบ | ผลกระทบถ้าผิด |
|---|---|---|---|
| A1 | ผู้ใช้เป้าหมายระยะกลางรวมถึง QC Operator ไม่ใช่แค่ ML Engineer | ถามทีม/หัวหน้าโดยตรง | Phase 4 ทั้งหมด (T-12 ถึง T-14) ไม่จำเป็น |
| A2 | ปัญหา `defect` เกิดจาก resolution/scale ไม่ใช่ diversity | T-01 + T-08 | ลำดับ T-08 ก่อน T-11 ผิด ต้องสลับ |
| A3 | Pre-annotation (T-05) จะลดเวลา label จริง | วัด median time per label ก่อน/หลัง | เสีย effort ไปกับฟีเจอร์ที่ไม่ช่วย |
| A4 | ผู้ใช้ยอมเสียเวลาเตรียม test set ถ้าเห็นประโยชน์ชัด | สังเกตว่ามีกี่ session ที่ข้ามขั้น evaluate | ต้องออกแบบวิธีวัดผลที่ไม่ต้องพึ่ง test set |
| A5 | ภาพบนสายพานมีความต่อเนื่องพอที่ copy กล่องจากภาพก่อนหน้า (T-10) จะช่วยได้ | ดูตัวอย่างภาพจริงจาก dataset | T-10 ไม่คุ้มทำ |
| A6 | ~~จำนวนผู้ใช้พร้อมกันยังน้อย (1 คนต่อโปรเจกต์/`input_dir`)~~ | ~~ถามทีม~~ | **ตัดสินใจแล้ว (2026-08-21): ผิด — ทีมต้องการรองรับหลายคนพร้อมกันจริง** พร้อมแผน login + workspace ในอนาคต → Phase 6 ([DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md)) ถูกเลื่อนขึ้นมาแล้ว |

### คำถามที่ทีมต้องตัดสินใจก่อนเริ่ม Phase 4

1. ~~จะเปิดให้หลายคนใช้โปรเจกต์ (`input_dir`) เดียวกันพร้อมกันหรือไม่~~ **ตัดสินใจแล้ว (2026-08-21): ใช่** — แก้ด้วยการย้าย label metadata ไป PostgreSQL (DB transaction ล็อกระดับแถวแทน file lock) ไม่ใช่ระบบ merge หลายคนแบบ real-time — ดู [DB_MIGRATION_PLAN.md](./DB_MIGRATION_PLAN.md) หัวข้อ 4.1 · ระบบ login/workspace เต็มรูปแบบยังเป็นงานแยกในอนาคต ไม่ใช่รอบนี้
2. **จะรองรับ QC Operator จริงหรือจะให้ ML Engineer เป็นตัวกลางต่อไป** — ตัดสินใจข้อนี้ก่อนลงทุน T-13/T-14
3. **เกณฑ์ F1 เท่าไหร่ถึงถือว่า "พอ" สำหรับ auto-label** — ต้องมีตัวเลขที่ตกลงกันไว้ ไม่งั้น FR-27 (คำเตือน) ตั้ง threshold ไม่ได้

---

## ภาคผนวก — สรุปสถานะแบบย่อ

**ทำเสร็จแล้ว (พร้อมใช้งาน):** core labeling loop ครบวงจร (label → bank → rescore → evaluate → auto-label → review), YOLO label format, class-index stability, test set แยกขาดจาก bank, background job pattern, path safety, file lock, เลือกโมเดล YOLOE ได้หลายเวอร์ชัน/ขนาด (FR-36) ล็อกต่อ bank

**ช่องว่างที่กระทบผู้ใช้มากที่สุด 5 อันดับแรก:**
1. ~~ไม่มีสัญญาณว่า "label พอหรือยัง" / คลาสตันแล้วหรือยัง (T-07)~~ ✅ แท็บ Progress + คำเตือน plateau
2. ~~ต้องวาดกล่องเองทุกภาพ ทั้งที่โมเดลทำนายให้ได้ (T-05)~~ ✅ pre-annotation
3. **คลาส `defect` ยังไม่ถึงเกณฑ์ auto-label (T-08)** — ช่องว่างอันดับ 1 แต่ตอนนี้รู้สาเหตุแล้ว: T-01 ชี้ว่าเป็นเรื่อง threshold + scale ไม่ใช่ "ไม่มีสัญญาณ" · `conf_by_class` ดึงจาก F1 0.00 → 0.248 แล้ว ที่เหลือต้องพึ่ง crop-before-SAVPE
4. ~~ไม่มี keyboard shortcut สำหรับงานซ้ำ (T-03)~~ ✅
5. ~~แก้ auto-label แล้วไม่ช่วยสอนโมเดล แต่ UI ไม่บอก (T-06)~~ ✅

**ช่องว่างที่กระทบการขยายผู้ใช้:** auth และ upload มี backend ครบแล้วแต่ยังไม่มี UI — เหลือหน้า login + dropzone เท่านั้น (ศัพท์เทคนิคแก้ไปแล้วด้วย Plain language mode)

**ช่องว่างเชิง infra:** job tracker หลาย worker (T-15), HTTPS (ต้องพึ่ง reverse proxy) · CI, non-root container, และ GPU support เขียนแล้วและ **build image ยืนยันสำเร็จจริงในเซสชันนี้** — runtime (`docker compose up` + GPU ในคอนเทนเนอร์ที่รันจริง) ยังไม่ทันยืนยันเพราะ Docker Desktop daemon ค้างหลัง build เสร็จ (ดู NFR-09/NFR-07)
