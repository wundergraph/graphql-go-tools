import http from 'k6/http';
import { check } from "k6";

export let options = {
	vus: 100,
	duration: "30s"
};

export default function() {
	let req, res;
	req = [{
		"method": "POST",
		"url": "http://0.0.0.0:8888/query",
		"body": "{\"operationName\":null,\"variables\":{},\"query\":\"{ documents { owner sensitiveInformation }}\"}",
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
