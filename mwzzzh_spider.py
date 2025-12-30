import asyncio
import logging
import json
import os
import random
import traceback
import time
from itertools import cycle
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Any

# 你的核心请求库
from curl_cffi.requests import AsyncSession

from device_register.dgmain2.register_logic import run_registration_flow


# ================= 1. 配置区域 =================
class Config:
    # 网络并发数 (Async Semaphores)
    MAX_CONCURRENCY = 200

    # 线程池大小 (建议设置为 CPU 核心数 * 1 到 2 倍)
    # 如果你的解析逻辑特别重，可以开大一点，比如 16 或 32
    THREAD_POOL_SIZE = 16

    PROXIES = [
    ]

    RESULT_FILE = "results12_21_5.jsonl"
    ERROR_FILE = "error.log"


# ================= 2. 日志系统 =================
logger = logging.getLogger("mwzzzh_spider")
logger.setLevel(logging.INFO)
formatter = logging.Formatter('%(asctime)s - [%(threadName)s] - %(message)s')

ch = logging.StreamHandler()
ch.setFormatter(formatter)
logger.addHandler(ch)

fh = logging.FileHandler(Config.ERROR_FILE, encoding='utf-8')
fh.setLevel(logging.ERROR)
fh.setFormatter(formatter)
logger.addHandler(fh)


# ================= 3. 数据管道 (保持不变) =================
class DataPipeline:
    def __init__(self, filename):
        self.filename = filename
        self.queue = asyncio.Queue()
        self.executor = ThreadPoolExecutor(max_workers=1)
        self.running = True
        self._writer_task = None

    async def start(self):
        self._writer_task = asyncio.create_task(self._consumer())

    async def save(self, data: Dict):
        await self.queue.put(data)

    def _write_impl(self, batch):
        try:
            with open(self.filename, 'a', encoding='utf-8') as f:
                for item in batch:
                    line = json.dumps(item, ensure_ascii=False)
                    f.write(line + "\n")
        except Exception as e:
            logger.error(f"写入失败: {e}")

    async def _consumer(self):
        batch = []
        while self.running or not self.queue.empty():
            try:
                item = await asyncio.wait_for(self.queue.get(), timeout=1.0)
                batch.append(item)
                self.queue.task_done()
            except asyncio.TimeoutError:
                pass
            if len(batch) >= 20 or (not self.running and batch):
                to_write = batch[:]
                batch.clear()
                await asyncio.get_event_loop().run_in_executor(
                    self.executor, self._write_impl, to_write
                )

    async def stop(self):
        self.running = False
        await self.queue.join()
        await self._writer_task
        self.executor.shutdown()


# ================= 4. 核心：同步解析逻辑 (在线程中运行) =================
'''
def sync_parsing_logic(type,resp,device, *args):
    """
    【注意】这是一个普通的 def，不是 async def。
    这里放所有 CPU 密集型操作：
    1. 正则 / XPath / PyQuery 解析
    2. 复杂的解密算法 (AES/RSA...)
    3. 数据清洗
    """
    # 模拟一个耗时的解析过程
    # 如果这段代码在 async def 里直接跑，会卡死整个爬虫
    # 但现在它在线程池里跑，所以不会影响网络请求
    # time.sleep(0.1) # 模拟 CPU 计算耗时
    # 假设解析出了结果
    # parsed_data = {
    #     "id": task_id,
    #     "title": f"Title found in length {len(html_content)}",
    #     "extra_calc": sum(i for i in range(10000))  # 模拟计算
    # }
    return parse_logic(type,resp,device, *args)

这里就直接注释掉了，因为现在我们把解析逻辑直接放倒了主类里面去
'''


# ================= 5. 核心：异步业务流程 (在主循环运行) =================
async def user_custom_logic(task_id, task_params, proxy, pipeline, thread_pool):
    """
    这里负责指挥：
    1. 遇到 IO (网络) -> await curl_cffi
    2. 遇到 CPU (计算) -> run_in_executor (扔给线程)
    """
    async with AsyncSession(impersonate="chrome131_android") as session:
        try:
            # 直接把 thread_pool 传给业务层
            device1 = await run_registration_flow(session, proxy, thread_pool, task_id)
            if type(device1)==dict:
                await pipeline.save(device1)
                logger.info(f"[{task_id}] 注册成功")
            else:
                logger.warning(f"[{task_id}] 注册失败 (返回 None),{device1}")

        except Exception as e:
            logger.error(f"[{task_id}] 致命报错: {e}")
            logger.error(traceback.format_exc())


# ================= 6. 引擎 =================
class SpiderEngine:
    def __init__(self):
        self.proxy_cycle = cycle(Config.PROXIES)
        self.pipeline = DataPipeline(Config.RESULT_FILE)
        self.sem = asyncio.Semaphore(Config.MAX_CONCURRENCY)

        # 【新增】计算型线程池
        # max_workers 决定了同一时刻最多有多少个解析任务在跑
        self.cpu_pool = ThreadPoolExecutor(max_workers=Config.THREAD_POOL_SIZE, thread_name_prefix="CpuWorker")

    def get_proxy(self):
        return next(self.proxy_cycle)

    async def _worker_wrapper(self, task_id, task_params):
        async with self.sem:
            try:
                # 把线程池传进去
                await user_custom_logic(task_id, task_params, self.get_proxy(), self.pipeline, self.cpu_pool)
            except Exception as e:
                logger.error(f"Wrapper error: {e}")

    async def run(self):
        await self.pipeline.start()

        # 生成任务
        tasks_data = [{"id": i} for i in range(1000)]
        logger.info(f"开始任务，网络并发: {Config.MAX_CONCURRENCY}, 解析线程: {Config.THREAD_POOL_SIZE}")

        coroutines = []
        for i, params in enumerate(tasks_data):
            task = asyncio.create_task(self._worker_wrapper(i, params))
            coroutines.append(task)

        # 等待完成
        try:
            from tqdm.asyncio import tqdm
            _ = [await f for f in tqdm.as_completed(coroutines)]
        except ImportError:
            await asyncio.gather(*coroutines)

        logger.info("任务完成，清理中...")

        # 关闭线程池
        self.cpu_pool.shutdown()
        await self.pipeline.stop()


if __name__ == "__main__":
    t = time.time()
    import sys

    # 【修正】代理加载逻辑
    proxy_file_path = "proxies.txt"  # 替换为你的真实路径

    if os.path.exists(proxy_file_path):
        with open(proxy_file_path, "r", encoding="utf-8") as f:
            # 过滤空行和空白字符
            Config.PROXIES = [line.strip() for line in f if line.strip()]
        print(f"已加载 {len(Config.PROXIES)} 个代理")
    else:
        print(f"警告：未找到代理文件 {proxy_file_path}，将使用空列表")
        # 这里可以放几个测试代理
        Config.PROXIES = []

    if not Config.PROXIES:
        print("错误：代理列表为空，程序退出")
        sys.exit(1)

    if sys.platform.startswith('win'):
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    engine = SpiderEngine()
    asyncio.run(engine.run())
    t1 = time.time()
    print("总耗时===>",t1-t)