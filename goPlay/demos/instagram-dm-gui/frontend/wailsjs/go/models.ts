export namespace config {
	
	export class AppConfig {
	    language: string;
	    card_code: string;
	    account_file: string;
	    target_file: string;
	    proxy_file: string;
	    success_file: string;
	    failure_file: string;
	    thread_title: string;
	    msg_content: string;
	    group_min: number;
	    group_max: number;
	    concurrency: number;
	    retry_count: number;
	    interval: number;
	    max_dm_count: number;
	    max_proxy_usage: number;
	    max_dm_per_account: number;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.card_code = source["card_code"];
	        this.account_file = source["account_file"];
	        this.target_file = source["target_file"];
	        this.proxy_file = source["proxy_file"];
	        this.success_file = source["success_file"];
	        this.failure_file = source["failure_file"];
	        this.thread_title = source["thread_title"];
	        this.msg_content = source["msg_content"];
	        this.group_min = source["group_min"];
	        this.group_max = source["group_max"];
	        this.concurrency = source["concurrency"];
	        this.retry_count = source["retry_count"];
	        this.interval = source["interval"];
	        this.max_dm_count = source["max_dm_count"];
	        this.max_proxy_usage = source["max_proxy_usage"];
	        this.max_dm_per_account = source["max_dm_per_account"];
	    }
	}

}

export namespace main {
	
	export class FileInfo {
	    name: string;
	    path: string;
	    line_count: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.line_count = source["line_count"];
	        this.size = source["size"];
	    }
	}

}

