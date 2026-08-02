export namespace main {
	
	export class Channel {
	    id: number;
	    platform: string;
	    name: string;
	    token: string;
	    desc: string;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.platform = source["platform"];
	        this.name = source["name"];
	        this.token = source["token"];
	        this.desc = source["desc"];
	        this.connected = source["connected"];
	    }
	}
	export class ChannelInput {
	    platform: string;
	    name: string;
	    token: string;
	    desc: string;
	
	    static createFrom(source: any = {}) {
	        return new ChannelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.name = source["name"];
	        this.token = source["token"];
	        this.desc = source["desc"];
	    }
	}
	export class Config {
	    apiKey: string;
	    model: string;
	    baseURL: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.baseURL = source["baseURL"];
	    }
	}
	export class CronInput {
	    name: string;
	    expr: string;
	    desc: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CronInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.expr = source["expr"];
	        this.desc = source["desc"];
	        this.enabled = source["enabled"];
	    }
	}
	export class CronJob {
	    id: number;
	    name: string;
	    expr: string;
	    desc: string;
	    enabled: boolean;
	    nextRun: string;
	
	    static createFrom(source: any = {}) {
	        return new CronJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.expr = source["expr"];
	        this.desc = source["desc"];
	        this.enabled = source["enabled"];
	        this.nextRun = source["nextRun"];
	    }
	}
	export class FileInfo {
	    name: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.icon = source["icon"];
	    }
	}
	export class Message {
	    type: string;
	    content: string;
	    time: string;
	    tool_name?: string;
	    tool_args?: string;
	    is_error?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.time = source["time"];
	        this.tool_name = source["tool_name"];
	        this.tool_args = source["tool_args"];
	        this.is_error = source["is_error"];
	    }
	}
	export class Skill {
	    id: number;
	    name: string;
	    desc: string;
	    keywords: string;
	    enabled: boolean;
	    fileName: string;
	    filePath: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.keywords = source["keywords"];
	        this.enabled = source["enabled"];
	        this.fileName = source["fileName"];
	        this.filePath = source["filePath"];
	    }
	}
	export class SkillInput {
	    name: string;
	    desc: string;
	    keywords: string;
	    fileContent: string;
	    fileName: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.keywords = source["keywords"];
	        this.fileContent = source["fileContent"];
	        this.fileName = source["fileName"];
	    }
	}
	export class Task {
	    id: number;
	    title: string;
	    subtitle: string;
	    time: string;
	    steps: number;
	    status: string;
	    messages?: Message[];
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.subtitle = source["subtitle"];
	        this.time = source["time"];
	        this.steps = source["steps"];
	        this.status = source["status"];
	        this.messages = this.convertValues(source["messages"], Message);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TodoItem {
	    id: number;
	    text: string;
	    completed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TodoItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.completed = source["completed"];
	    }
	}

}

