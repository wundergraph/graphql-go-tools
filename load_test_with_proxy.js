import http from 'k6/http';
import { check } from "k6";

export let options = {
	vus: 50,
	duration: "30s"
};

export default function() {
	let req, res;
	req = [{
		"method": "POST",
		"url": "http://localhost:8888/query",
		"body": "{\"operationName\":null,\"variables\":{},\"query\":\"{\\n  documents{\\n    owner\\n    sensitiveInformation\\n  }\\n}\\n\"}",
		"params": {
			"headers": {
				"user":"jens"
			}
		}
	}];
	res = http.batch(req);

	check(res[0], {
		"is status 200": (r) => r.status === 200
	});
}
