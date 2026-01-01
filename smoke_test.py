import os
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent


def run(cmd: list[str], cwd: Path | None = None) -> None:
    print(f"\n$ {' '.join(cmd)}")
    p = subprocess.run(cmd, cwd=str(cwd or ROOT), capture_output=False)
    if p.returncode != 0:
        raise SystemExit(p.returncode)


def main() -> None:
    # 1) Python 依赖与脚本自检（生成设备脚本）
    #    全 DB 模式：只验证能生成+写文件
    out_dir = ROOT / "smoke_device_backups"
    out_dir.mkdir(exist_ok=True)

    run(
        [
            sys.executable,
            "generate_devices_bulk.py",
            "--save-file",
            "--max-generate",
            "50",
            "--per-file-max",
            "10",
            "--out-dir",
            str(out_dir),
            "--file-prefix",
            "devices_smoke",
            "--concurrency",
            "20",
        ]
    )

    # 2) Python 注册主流程导入自检（不实际跑网络任务）
    run([sys.executable, "-c", "import mwzzzh_spider; print('mwzzzh_spider import ok')"])

    # 3) Go stats 项目编译自检（确保全 DB 改动后可构建）
    go_mod_dir = ROOT / "goPlay"
    run(["go", "test", "./demos/stats/dgmain3", "-c"], cwd=go_mod_dir)

    print("\n[OK] smoke_test passed")


if __name__ == "__main__":
    main()


