## Python 依赖安装（本仓库根目录）

### 1) 创建虚拟环境（推荐）

Windows PowerShell:

```powershell
python -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install -U pip
```

> 强烈建议使用虚拟环境：避免你电脑里其它 Python 包（例如已安装的某些工具包）和本项目依赖产生版本冲突。

### 2) 安装依赖

```powershell
pip install -r requirements.txt
```

### 3) 快速自检（确保依赖齐全）

```powershell
python -c "import curl_cffi,requests,urllib3,Crypto,cryptography,gmssl,ecdsa,google.protobuf,tqdm; print('OK')"
```

> 说明：
> - `Crypto` 来自 `pycryptodome`
> - `google.protobuf` 来自 `protobuf`


