// 自定义 combo：短名 label + 灰色完整 QN 副文本（两行式，决策 6 方案 B）+ 违规计数角标（决策 8）。
// 基类 RectCombo 只画 key + 单一 label；override drawLabelShape 追加 qn-label 形状。
// 10.1.4：移除 ✕ 折叠按钮（有漂移 bug），折叠改右键菜单触发；右上角让位给违规计数角标。
import { Rect, Text } from '@antv/g'
import { RectCombo, register } from '@antv/g6'
import type { DisplayObjectConfig, Group } from '@antv/g'
import type { RectComboStyleProps } from '@antv/g6'

interface ComboModelData {
  qn?: string
  mainCombo?: boolean
  /** 违规向上汇聚计数（决策 8 / 11.17）：cycle=子树内环组数 / fanin=子树内高扇入类数 / fanout=子树内高扇出类数。 */
  violation?: { cycle: number; fanin?: number; fanout: number }
}

class QnCombo extends RectCombo {
  constructor(options: DisplayObjectConfig<RectComboStyleProps>) {
    super(options)
  }

  protected drawLabelShape(attributes: Required<RectComboStyleProps>, container: Group): void {
    // 主 label：短名（父级逻辑）。
    super.drawLabelShape(attributes, container)
    // 业务数据在 model 的 combo data 上（shape attributes 里没有），读 model。
    let model: ComboModelData = {}
    try {
      const items = this.context.model.getComboData([this.id as string])
      model = (items?.[0]?.data as ComboModelData | undefined) ?? {}
    } catch {
      model = {}
    }
    const key = this.getShape('key')
    const keyBBox = key?.getBBox?.()
    const keyH = keyBBox ? keyBBox.height : 32
    const keyW = keyBBox ? keyBBox.width : 64
    // 副文本：完整 QN（model data.qn），灰色小字，放主 label 上方。
    const qn = model.qn ?? ''
    if (qn) {
      const y = -(keyH / 2) - 24 // key 顶边上方预留短名 label（约 14px）+ 间距
      this.upsert(
        'qn-label',
        Text,
        {
          text: qn,
          fill: '#86909c',
          fontSize: 9,
          textAlign: 'center',
          textBaseline: 'bottom',
          fontFamily: 'system-ui, sans-serif',
          pointerEvents: 'none',
          y,
        } as never,
        container,
      )
    }
    // 右上角角标列（10.5 重构）：kind 徽标（包）在上，违规计数角标（[▲N环]红 / [⇧N高扇出]橙）在下。
    // 统一胶囊样式（radius 8，与 node kind 徽标一致），紧贴包右上角，违规角标更明显。
    this.drawCornerBadges(model.violation ?? null, keyW, keyH, container)
  }

  /** 右上角角标列：kind 徽标「包」始终显示；违规计数角标（环/高扇出）排在其下。
   *  可点击（name=corner-badge，combo-click 事件）。
   *  11.1 问题4：徽标尺寸随 combo 框大小缩放（keyH 大 → 徽标大；keyH 小 → 徽标小）。
   *  11.2 用户要求：包 kind 徽标样式 = 类 kind 徽标样式（对齐 ProtoGraphCanvas node badges）——
   *  kind-badge 字号 fs∈[7,10]、小圆角 radius=fs+1、垂直 padding 1（h=fs+2）、描边 1px；
   *  违规角标保持 10.5「更明显」定位（大一号胶囊 + 加粗描边 1.5px），两套参数分离。 */
  private drawCornerBadges(v: { cycle: number; fanin?: number; fanout: number } | null, keyW: number, keyH: number, container: Group): void {
    // kind 徽标：随框高缩放但与 node badge 同范围同形态（原 fs 8-13 比类徽标大一号）。
    const fsK = Math.max(7, Math.min(10, Math.round(5 + keyH * 0.03)))
    const hK = fsK + 2 // node badge 垂直 padding [1,1] → 高 = 1+fs+1
    const padK = Math.max(1, Math.round(fsK * 0.45))
    const radK = fsK + 1 // 与 node badge backgroundRadius 一致（小圆角，非全圆角）
    // 违规角标：10.5 决策「更明显」，保留大一号胶囊 + 加粗描边。
    const fsV = Math.max(8, Math.min(13, Math.round(6 + keyH * 0.05)))
    const hV = fsV + 8
    const padV = Math.round(fsV * 0.55)
    const radV = Math.round(hV / 2)
    const items = [
      { key: 'kind-badge', text: '包', bg: '#eef1f4', fg: '#4e5969', border: '#d5dae0', fs: fsK, h: hK, pad: padK, radius: radK, lw: 1 },
      ...(v && v.cycle > 0 ? [{ key: 'vio-cycle', text: `▲${v.cycle}环`, bg: '#fff1f0', fg: '#cf1322', border: '#e8684a', fs: fsV, h: hV, pad: padV, radius: radV, lw: 1.5 }] : []),
      // 11.17：高扇入/高扇出可同时存在，分别成角标（原只有高扇出）。
      ...(v && (v.fanin ?? 0) > 0 ? [{ key: 'vio-fanin', text: `⇩${v.fanin}高扇入`, bg: '#fff0e6', fg: '#d4380d', border: '#F77234', fs: fsV, h: hV, pad: padV, radius: radV, lw: 1.5 }] : []),
      ...(v && v.fanout > 0 ? [{ key: 'vio-fanout', text: `⇧${v.fanout}高扇出`, bg: '#e6f7ee', fg: '#237804', border: '#2f9e6e', fs: fsV, h: hV, pad: padV, radius: radV, lw: 1.5 }] : []),
    ]
    // 11.3 修复：文字被胶囊盖住。@antv/g 同父下后插入者在上层，原「先 Text 量宽、后 Rect 背景」
    // 使胶囊盖住「包」字；且 upsert 只在首次创建时 appendChild，之后走 update 不重排，toFront 在
    // G6 绘制管线中不可靠。方案：用独立临时 Text（__measure-*）量宽（量完即删），主 Text 与 Rect
    // 按「Rect 先、Text 后」顺序创建——保证 Text 插入顺序在 Rect 之后、天然在上层。
    let y = -keyH / 2 + 2
    const layouts: { key: string; b: typeof items[number]; w: number; x: number; y: number }[] = []
    for (const b of items) {
      // 临时 Text 量宽（独立 className，量完删除，不参与主 Text/Rect 的顺序）。
      const mKey = '__measure-' + b.key
      const mt = this.upsert(
        mKey,
        Text,
        {
          text: b.text,
          fill: b.fg,
          fontSize: b.fs,
          fontFamily: 'system-ui, sans-serif',
          pointerEvents: 'none',
          x: 0,
          y: 0,
        } as never,
        container,
      )
      const tw = mt?.getBBox().width ?? 0
      this.upsert(mKey, Text, false as never, container) // 量完即删
      const w = tw + b.pad * 2
      const x = keyW / 2 - w + 3 // 右上角，右对齐，紧贴包边
      layouts.push({ key: b.key, b, w, x, y })
      y += b.h + 3
    }
    // 先 Rect（背景）…
    for (const L of layouts) {
      this.upsert(
        L.key,
        Rect,
        {
          width: L.w,
          height: L.b.h,
          x: L.x,
          y: L.y,
          radius: L.b.radius,
          fill: L.b.bg,
          stroke: L.b.border,
          lineWidth: L.b.lw,
          cursor: 'pointer',
          pointerEvents: 'all',
          name: 'corner-badge',
        } as never,
        container,
      )
    }
    // …再 Text（前景，插入在后/上层），文字水平垂直居中于胶囊。
    for (const L of layouts) {
      this.upsert(
        L.key + '-t',
        Text,
        {
          text: L.b.text,
          fill: L.b.fg,
          fontSize: L.b.fs,
          textAlign: 'center',
          textBaseline: 'middle',
          fontFamily: 'system-ui, sans-serif',
          pointerEvents: 'none',
          x: L.x + L.w / 2, // 11.1 问题4：水平居中
          y: L.y + L.b.h / 2, // 垂直居中（原滞留 combo 中心水平线）
        } as never,
        container,
      )
    }
  }
}

// 注册一次。
let registered = false
export function registerQnCombo(): void {
  if (registered) return
  register('combo', 'qn-combo', QnCombo)
  registered = true
}
