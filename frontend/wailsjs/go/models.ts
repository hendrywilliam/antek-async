export namespace kube {
	
	export class Candidate {
	    source: string;
	    path: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Candidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.path = source["path"];
	        this.exists = source["exists"];
	    }
	}
	export class PodInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    status: string;
	    restarts: number;
	    age: string;
	    node: string;
	
	    static createFrom(source: any = {}) {
	        return new PodInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.status = source["status"];
	        this.restarts = source["restarts"];
	        this.age = source["age"];
	        this.node = source["node"];
	    }
	}
	export class Status {
	    source: string;
	    path: string;
	    context: string;
	    cluster: string;
	    server: string;
	    candidates: Candidate[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.path = source["path"];
	        this.context = source["context"];
	        this.cluster = source["cluster"];
	        this.server = source["server"];
	        this.candidates = this.convertValues(source["candidates"], Candidate);
	        this.error = source["error"];
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

}

export namespace main {
	
	export class AppState {
	    config: kube.Status;
	    pods: kube.PodInfo[];
	    connected: boolean;
	    updatedAt: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], kube.Status);
	        this.pods = this.convertValues(source["pods"], kube.PodInfo);
	        this.connected = source["connected"];
	        this.updatedAt = source["updatedAt"];
	        this.error = source["error"];
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

}

