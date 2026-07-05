export namespace updater {
	
	export class BackupEntry {
	    name: string;
	    version: string;
	    component: string;
	    backup_path: string;
	    // Go type: time
	    created_at: any;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.component = source["component"];
	        this.backup_path = source["backup_path"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.size = source["size"];
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
	export class IgnoredVersion {
	    component: string;
	    version: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new IgnoredVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.component = source["component"];
	        this.version = source["version"];
	        this.reason = source["reason"];
	    }
	}

}

