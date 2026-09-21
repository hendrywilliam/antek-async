export namespace kube {
	
	export class ApplyResult {
	    apiVersion: string;
	    kind: string;
	    namespace: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	    }
	}
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
	export class DeploymentInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    upToDate: number;
	    available: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.upToDate = source["upToDate"];
	        this.available = source["available"];
	        this.age = source["age"];
	    }
	}
	export class NodeInfo {
	    name: string;
	    status: string;
	    roles: string;
	    version: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.roles = source["roles"];
	        this.version = source["version"];
	        this.age = source["age"];
	    }
	}
	export class PodInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    status: string;
	    restarts: number;
	    age: string;
	    ip: string;
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
	        this.ip = source["ip"];
	        this.node = source["node"];
	    }
	}
	export class StatefulSetInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.age = source["age"];
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
	
	export class NodesState {
	    items: kube.NodeInfo[];
	    loaded: boolean;
	    loading: boolean;
	    error: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new NodesState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], kube.NodeInfo);
	        this.loaded = source["loaded"];
	        this.loading = source["loading"];
	        this.error = source["error"];
	        this.updatedAt = source["updatedAt"];
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
	export class StatefulSetsState {
	    items: kube.StatefulSetInfo[];
	    loaded: boolean;
	    loading: boolean;
	    error: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetsState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], kube.StatefulSetInfo);
	        this.loaded = source["loaded"];
	        this.loading = source["loading"];
	        this.error = source["error"];
	        this.updatedAt = source["updatedAt"];
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
	export class DeploymentsState {
	    items: kube.DeploymentInfo[];
	    loaded: boolean;
	    loading: boolean;
	    error: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentsState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], kube.DeploymentInfo);
	        this.loaded = source["loaded"];
	        this.loading = source["loading"];
	        this.error = source["error"];
	        this.updatedAt = source["updatedAt"];
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
	export class PodsState {
	    items: kube.PodInfo[];
	    loaded: boolean;
	    loading: boolean;
	    error: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PodsState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], kube.PodInfo);
	        this.loaded = source["loaded"];
	        this.loading = source["loading"];
	        this.error = source["error"];
	        this.updatedAt = source["updatedAt"];
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
	export class AppState {
	    config: kube.Status;
	    active: string;
	    pods: PodsState;
	    deployments: DeploymentsState;
	    statefulSets: StatefulSetsState;
	    nodes: NodesState;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], kube.Status);
	        this.active = source["active"];
	        this.pods = this.convertValues(source["pods"], PodsState);
	        this.deployments = this.convertValues(source["deployments"], DeploymentsState);
	        this.statefulSets = this.convertValues(source["statefulSets"], StatefulSetsState);
	        this.nodes = this.convertValues(source["nodes"], NodesState);
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

