export namespace main {
	
	export class CreateBundleRequest {
	    display_name: string;
	    peer_id: string;
	    public_key_hex: string;
	    admin_passphrase: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateBundleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.display_name = source["display_name"];
	        this.peer_id = source["peer_id"];
	        this.public_key_hex = source["public_key_hex"];
	        this.admin_passphrase = source["admin_passphrase"];
	    }
	}
	export class PeerInfo {
	    id: string;
	    display_name: string;
	    verified: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.display_name = source["display_name"];
	        this.verified = source["verified"];
	    }
	}
	export class NodeStatus {
	    state: string;
	    peer_id: string;
	    display_name: string;
	    is_running: boolean;
	    connected_peers: PeerInfo[];
	
	    static createFrom(source: any = {}) {
	        return new NodeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.peer_id = source["peer_id"];
	        this.display_name = source["display_name"];
	        this.is_running = source["is_running"];
	        this.connected_peers = this.convertValues(source["connected_peers"], PeerInfo);
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
	export class OnboardingInfo {
	    peer_id: string;
	    public_key_hex: string;
	
	    static createFrom(source: any = {}) {
	        return new OnboardingInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peer_id = source["peer_id"];
	        this.public_key_hex = source["public_key_hex"];
	    }
	}

}

