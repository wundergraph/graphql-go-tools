import http from 'k6/http';
import { check } from "k6";

export let options = {
    vus: 50,
	duration: "10s"
};

export default function() {
	let req, res;
	req = [{
		"method": "POST",
		"url": "http://localhost:8889/query",
		"body": "{\"operationName\":null,\"variables\":{},\"query\":\"{\\n  documents(user: \\\"jens\\\") {\\n    owner\\n    sensitiveInformation\\n  }\\n}\\n\"}",
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
