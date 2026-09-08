<template>
  <div class="guide">
    <el-card shadow="never" class="g-card">
      <template #header>
        <div class="g-title">
          <el-icon :size="22"><Reading /></el-icon>
          <span>系统使用手册</span>
        </div>
      </template>
      <p class="g-intro">
        技术部门会议系统用于「会议准备 → 汇报展示 → 纪要归档」的全流程管理：管理员组织会议，
        各部门/小组负责人提前录入汇报内容，会上以 PPT 形式逐条展示，会后自动生成并归档会议纪要。
      </p>
    </el-card>

    <!-- 角色分工 -->
    <el-card shadow="never" class="g-card">
      <template #header>一、角色分工</template>
      <el-table :data="roles" border size="small">
        <el-table-column prop="role" label="角色" width="130" />
        <el-table-column prop="scope" label="可以做什么" />
      </el-table>
    </el-card>

    <!-- 会议全流程 -->
    <el-card shadow="never" class="g-card">
      <template #header>二、会议全流程（状态流转）</template>
      <el-steps direction="vertical" :active="4" class="g-steps">
        <el-step title="① 筹备中 draft" description="管理员创建会议、设置参会组织；各组织负责人录入 / 修改 / 删除本组织汇报事项，可上传附件；管理员可多次生成材料（会前预览）。" />
        <el-step title="② 进行中 ongoing" description="管理员点击「开始会议」后进入会议展示页，按组织逐条展示汇报（全屏/键盘翻页）；会议期间可在展示页为每条汇报补充讨论结论或布置新任务。" />
        <el-step title="③ 已结束 finished" description="管理员点击「结束会议」，汇报事项进入只读；此时可正式生成会议材料（仅一次）与会议纪要，纪要仍可编辑。" />
        <el-step title="④ 已归档 archived" description="仅「已结束」的会议可归档。归档后材料与纪要保持只读，不能再修改；已配置企业微信微盘时，归档会同时自动上传纪要。" />
      </el-steps>
      <el-alert
        title="状态只能依次推进：筹备中 → 进行中 → 已结束 → 已归档。请按序操作，不要跳过「结束会议」直接归档。"
        type="info"
        :closable="false"
        show-icon
      />
    </el-card>

    <!-- 关键操作 -->
    <el-card shadow="never" class="g-card">
      <template #header>三、各阶段关键操作要点</template>
      <div class="g-block">
        <h4>1. 筹备阶段（会前 1~3 天建议完成）</h4>
        <ul>
          <li>管理员在「会议管理」创建会议，填写名称、时间、地点，并勾选<b>参会组织</b>（可含部门或小组）。</li>
          <li>参会组织可设为<b>保密会议</b>：保密会议仅管理员与参会组织负责人可见。</li>
          <li>部门/小组负责人在会议列表点「录入汇报」或直接进入会议详情录入，一条会议可录多条事项。</li>
          <li>汇报内容<b>支持 Markdown</b>：可用 <code>## 小标题</code>、<code>**加粗**</code>、<code>- 列表</code>、<code>| 表格 |</code> 等排版，会上展示更清晰；录入框内可切换「预览」查看效果。</li>
          <li>汇报事项支持上传附件（图片 / 视频 / 音频 / 文档），展示时可直接预览或下载。</li>
          <li>只有 <b>筹备中</b> 的会议才能录入与修改汇报，会议开始后不可再改动，请提前确认内容。</li>
        </ul>
      </div>
      <div class="g-block">
        <h4>2. 会议进行（会中）</h4>
        <ul>
          <li>管理员确认所有汇报录入完成后点「开始会议」，系统自动进入<b>会议展示</b>（PPT 式全屏）。</li>
          <li>展示页支持键盘：<kbd>→</kbd>/<kbd>空格</kbd> 下一项、<kbd>←</kbd> 上一项、<kbd>Esc</kbd> 退出。</li>
          <li>展示期间管理员可为每条汇报录入<b>讨论结论</b>或<b>新任务</b>（任务可指定负责人与截止时间）。</li>
        </ul>
      </div>
      <div class="g-block">
        <h4>3. 会后收尾</h4>
        <ul>
          <li>管理员点「结束会议」→ 再点「生成会议纪要」（可多次重新生成或手动编辑，会议进行中 / 已结束后均可）。</li>
          <li>材料规则：筹备中可多次生成（预览用）；会议开始后仅可正式生成一次。</li>
          <li>确认纪要内容无误后点「会议归档」，按提示确认。归档后<b>纪要不可修改</b>，请务必先核对。</li>
          <li>归档成功后可在详情页随时回看，或点「下载 .md」导出纪要。</li>
        </ul>
      </div>
    </el-card>

    <!-- 常见问题与限制 -->
    <el-card shadow="never" class="g-card">
      <template #header>四、常见限制与提示</template>
      <ul class="g-faq">
        <li><b>谁可以录汇报？</b> 管理员可为任意参会组织录入；部门/小组负责人只能录入本组织的事项，且仅限本组织参会的会议。</li>
        <li><b>谁可以管理？</b> 组织 / 账号 / 会议室管理与操作日志仅管理员可见；会议状态操作（开始 / 结束 / 归档）仅管理员可执行。</li>
        <li><b>忘了录汇报能开会吗？</b> 没有录入任何汇报内容的会议无法「开始」。</li>
        <li><b>归档后想改纪要怎么办？</b> 归档后纪要只读，请归档前完成核对；如确有需要可联系管理员在数据库中处理。</li>
        <li><b>密码忘了？</b> 登录后可在右上角头像菜单「修改密码」；忘记密码请联系管理员重置。</li>
        <li><b>一个组织有多个账号</b> 时可共用同一个组织录入，各账号互不影响。</li>
      </ul>
    </el-card>
  </div>
</template>

<script setup>
import { Reading } from '@element-plus/icons-vue'

const roles = [
  { role: '管理员', scope: '创建/管理会议、设置参会组织、开始/结束/归档会议、生成材料与纪要；管理组织架构、账号、会议室；查看操作日志。' },
  { role: '部门/小组负责人', scope: '为本人所属组织录入/修改汇报事项与附件（仅筹备中）；查看本组织参与的会议、材料与纪要。' },
  { role: '组织成员', scope: '与负责人一致，可为本组织录入汇报内容（由管理员分配账号与所属组织）。' },
]
</script>

<style scoped>
.guide {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
}
.g-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.g-intro {
  color: #606266;
  margin: 4px 0 0;
  line-height: 1.8;
}
.g-steps {
  margin-bottom: 16px;
}
.g-block {
  margin-bottom: 8px;
}
.g-block h4 {
  margin: 12px 0 6px;
  color: #1f2d3d;
}
.g-block ul,
.g-faq {
  margin: 6px 0;
  padding-left: 20px;
  line-height: 1.9;
  color: #303133;
}
.g-faq li {
  margin: 4px 0;
}
code {
  background: #f2f3f5;
  border-radius: 3px;
  padding: 1px 5px;
  color: #c7254e;
  font-size: 13px;
}
kbd {
  background: #f2f3f5;
  border: 1px solid #dcdfe6;
  border-bottom-width: 2px;
  border-radius: 3px;
  padding: 0 5px;
  font-size: 12px;
}
</style>
