export namespace main {
	
	export class AppPaths {
	    binPath: string;
	    modelsPath: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPaths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.binPath = source["binPath"];
	        this.modelsPath = source["modelsPath"];
	    }
	}
	export class CommandResult {
	    ok: boolean;
	    exitCode: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new CommandResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.exitCode = source["exitCode"];
	        this.error = source["error"];
	    }
	}
	export class ComparePair {
	    name: string;
	    before: string;
	    after: string;
	
	    static createFrom(source: any = {}) {
	        return new ComparePair(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.before = source["before"];
	        this.after = source["after"];
	    }
	}
	export class ModelInfo {
	    name: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	    }
	}
	export class UpscaleOptions {
	    binPath: string;
	    modelsPath: string;
	    inputPath: string;
	    outputPath: string;
	    modelName: string;
	    scale: number;
	    format: string;
	    tileSize: number;
	    gpuId: string;
	    compress: number;
	    tta: boolean;
	    verbose: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpscaleOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.binPath = source["binPath"];
	        this.modelsPath = source["modelsPath"];
	        this.inputPath = source["inputPath"];
	        this.outputPath = source["outputPath"];
	        this.modelName = source["modelName"];
	        this.scale = source["scale"];
	        this.format = source["format"];
	        this.tileSize = source["tileSize"];
	        this.gpuId = source["gpuId"];
	        this.compress = source["compress"];
	        this.tta = source["tta"];
	        this.verbose = source["verbose"];
	    }
	}

}

