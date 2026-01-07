export namespace config {
	
	export class AppConfig {
	    language: string;
	    card_code: string;
	    sms_file: string;
	    email_file: string;
	    cookie_path: string;
	    success_path: string;
	    failure_path: string;
	    two_factor_dir: string;
	    concurrency: number;
	    auto_2fa: boolean;
	    reg_mode: string;
	    push_url: string;
	    max_reg_count: number;
	    max_phone_usage: number;
	    font_size: number;
	    theme_color: string;
	    proxy_file: string;
	    max_success_per_file: number;
	    poll_timeout_sec: number;
	    sms_wait_timeout_sec: number;
	    finalize_retries: number;
	    enable_header_rotation: boolean;
	    enable_anomalous_ua: boolean;
	    enable_ios: boolean;
	    http_request_timeout_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.card_code = source["card_code"];
	        this.sms_file = source["sms_file"];
	        this.email_file = source["email_file"];
	        this.cookie_path = source["cookie_path"];
	        this.success_path = source["success_path"];
	        this.failure_path = source["failure_path"];
	        this.two_factor_dir = source["two_factor_dir"];
	        this.concurrency = source["concurrency"];
	        this.auto_2fa = source["auto_2fa"];
	        this.reg_mode = source["reg_mode"];
	        this.push_url = source["push_url"];
	        this.max_reg_count = source["max_reg_count"];
	        this.max_phone_usage = source["max_phone_usage"];
	        this.font_size = source["font_size"];
	        this.theme_color = source["theme_color"];
	        this.proxy_file = source["proxy_file"];
	        this.max_success_per_file = source["max_success_per_file"];
	        this.poll_timeout_sec = source["poll_timeout_sec"];
	        this.sms_wait_timeout_sec = source["sms_wait_timeout_sec"];
	        this.finalize_retries = source["finalize_retries"];
	        this.enable_header_rotation = source["enable_header_rotation"];
	        this.enable_anomalous_ua = source["enable_anomalous_ua"];
	        this.enable_ios = source["enable_ios"];
	        this.http_request_timeout_sec = source["http_request_timeout_sec"];
	    }
	}
	export class LanguagePack {
	    login_title: string;
	    login_desc: string;
	    card_code: string;
	    login_btn: string;
	    hardware_id: string;
	    expiry_date: string;
	    logout: string;
	    settings: string;
	    menu_execution: string;
	    menu_params: string;
	    menu_archives: string;
	    menu_app_settings: string;
	    lang_select: string;
	    stats_success: string;
	    stats_failed: string;
	    stats_total: string;
	    sms_mode: string;
	    email_mode: string;
	    run: string;
	    source_data: string;
	    output_config: string;
	    select_file: string;
	    save_path: string;
	    logs: string;
	    logs_clear: string;
	    logs_empty: string;
	    success_folder: string;
	    failure_folder: string;
	    cookie_folder: string;
	    two_factor_folder: string;
	    api_push_url: string;
	    max_reg_limit: string;
	    max_phone_usage: string;
	    save_config: string;
	    param_desc: string;
	    engine_perf: string;
	    engine_perf_desc: string;
	    parallel_workers: string;
	    auto_2fa_sec: string;
	    auto_2fa_desc: string;
	    task_safety: string;
	    task_safety_desc: string;
	    external_int: string;
	    external_int_desc: string;
	    storage_paths: string;
	    storage_paths_desc: string;
	    font_size: string;
	    theme_color: string;
	    success_path_title: string;
	    failure_path_title: string;
	    cookie_path_title: string;
	    under_dev: string;
	    proxy_source: string;
	    alert_task_completed: string;
	    alert_login_failed: string;
	    alert_select_file: string;
	    alert_settings_saved: string;
	    alert_settings_save_failed: string;
	    alert_api_success: string;
	    alert_api_failed: string;
	    confirm_clear_stats: string;
	    confirm_delete_file: string;
	
	    static createFrom(source: any = {}) {
	        return new LanguagePack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login_title = source["login_title"];
	        this.login_desc = source["login_desc"];
	        this.card_code = source["card_code"];
	        this.login_btn = source["login_btn"];
	        this.hardware_id = source["hardware_id"];
	        this.expiry_date = source["expiry_date"];
	        this.logout = source["logout"];
	        this.settings = source["settings"];
	        this.menu_execution = source["menu_execution"];
	        this.menu_params = source["menu_params"];
	        this.menu_archives = source["menu_archives"];
	        this.menu_app_settings = source["menu_app_settings"];
	        this.lang_select = source["lang_select"];
	        this.stats_success = source["stats_success"];
	        this.stats_failed = source["stats_failed"];
	        this.stats_total = source["stats_total"];
	        this.sms_mode = source["sms_mode"];
	        this.email_mode = source["email_mode"];
	        this.run = source["run"];
	        this.source_data = source["source_data"];
	        this.output_config = source["output_config"];
	        this.select_file = source["select_file"];
	        this.save_path = source["save_path"];
	        this.logs = source["logs"];
	        this.logs_clear = source["logs_clear"];
	        this.logs_empty = source["logs_empty"];
	        this.success_folder = source["success_folder"];
	        this.failure_folder = source["failure_folder"];
	        this.cookie_folder = source["cookie_folder"];
	        this.two_factor_folder = source["two_factor_folder"];
	        this.api_push_url = source["api_push_url"];
	        this.max_reg_limit = source["max_reg_limit"];
	        this.max_phone_usage = source["max_phone_usage"];
	        this.save_config = source["save_config"];
	        this.param_desc = source["param_desc"];
	        this.engine_perf = source["engine_perf"];
	        this.engine_perf_desc = source["engine_perf_desc"];
	        this.parallel_workers = source["parallel_workers"];
	        this.auto_2fa_sec = source["auto_2fa_sec"];
	        this.auto_2fa_desc = source["auto_2fa_desc"];
	        this.task_safety = source["task_safety"];
	        this.task_safety_desc = source["task_safety_desc"];
	        this.external_int = source["external_int"];
	        this.external_int_desc = source["external_int_desc"];
	        this.storage_paths = source["storage_paths"];
	        this.storage_paths_desc = source["storage_paths_desc"];
	        this.font_size = source["font_size"];
	        this.theme_color = source["theme_color"];
	        this.success_path_title = source["success_path_title"];
	        this.failure_path_title = source["failure_path_title"];
	        this.cookie_path_title = source["cookie_path_title"];
	        this.under_dev = source["under_dev"];
	        this.proxy_source = source["proxy_source"];
	        this.alert_task_completed = source["alert_task_completed"];
	        this.alert_login_failed = source["alert_login_failed"];
	        this.alert_select_file = source["alert_select_file"];
	        this.alert_settings_saved = source["alert_settings_saved"];
	        this.alert_settings_save_failed = source["alert_settings_save_failed"];
	        this.alert_api_success = source["alert_api_success"];
	        this.alert_api_failed = source["alert_api_failed"];
	        this.confirm_clear_stats = source["confirm_clear_stats"];
	        this.confirm_delete_file = source["confirm_delete_file"];
	    }
	}

}

export namespace main {
	
	export class ArchiveItem {
	    name: string;
	    path: string;
	    size: number;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.time = source["time"];
	    }
	}

}

