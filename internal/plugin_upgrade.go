package internal

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

// handlePluginUpgrade 执行 openclaw-channel-dmwork 插件升级
//   - npx -y openclaw-channel-dmwork install --force
//   - CLI 自身会：下载最新 npm 版本 → 安装到 openclaw extensions → 自动重启 openclaw gateway
//   - daemon 不主动上报 completed，靠 register handler 里的 plugin 关单路径关闭
//     (register 上报 metadata.plugins 含新版本 → 服务端 completeUpgradeIfMatchedWithRuntime 关单)
func (d *Daemon) handlePluginUpgrade(ctx context.Context, up *PendingUpgrade) {
	log.Printf("[INFO] plugin upgrade task: %s → %s (task=%s)", up.Component, up.TargetVersion, up.TaskID)

	// dispatched → installing
	d.reportUpgrade(ctx, up.TaskID, "installing", "")

	// 服务端插件 timeout 10 分钟；daemon 这里留 1 分钟 buffer 给上报，用 9 分钟
	installCtx, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()

	// v1 只装 latest，不传版本号；--force 避免 CLI 交互式确认
	cmd := exec.CommandContext(installCtx, "npx", "-y", up.Component, "install", "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := fmt.Sprintf("npx install failed: %v\noutput: %s", err, truncateOutput(string(out), 2000))
		log.Printf("[ERROR] plugin upgrade failed: %s", msg)
		d.reportUpgrade(ctx, up.TaskID, "failed", msg)
		return
	}
	log.Printf("[INFO] plugin upgrade npx exited cleanly (task=%s)", up.TaskID)

	// 主动触发 detect + register 加速关单，否则最多等 15s 心跳周期。
	// openclaw gateway restart 需要几秒，先等一下再探测避免扫到旧进程。
	go func() {
		time.Sleep(3 * time.Second)
		detectCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := d.fastDetectAndRegister(detectCtx); err != nil {
			log.Printf("[WARN] post-upgrade re-register failed: %v", err)
		}
	}()
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
