package mockapi

import (
	"fmt"
	"net/http"
	"strings"
)

// Phase 7 endpoints: buckets, storage keys, load balancers, DNS zones and records, and
// snapshot schedules. Like the rest of the fake they keep JSON objects and settle after a
// few GETs; only the rules the provider relies on are checked.

// VerifyZone plays the domain being delegated to us: the zone becomes active and served.
func (s *Server) VerifyZone(zonePath string) {
	s.Mutate(zonePath, func(o map[string]any) {
		o["state"], o["status"], o["served"], o["serial"], o["delegation_ok"] = "active", "active", true, 2026110701, true
		v := o["verification"].(map[string]any)
		v["state"], v["method"] = "verified", "ns"
	})
}

// SetObjects pretends a bucket holds objects (the real count comes from Ceph).
func (s *Server) SetObjects(bucketPath string, n int) {
	s.Mutate(bucketPath, func(o map[string]any) { o["object_count"] = n })
}

const s3Endpoint = "https://s3.neo.example"

var p7Collections = map[string]bool{
	"storage": true, "buckets": true, "storage-keys": true, "load-balancers": true,
	"snapshot-schedules": true, "dns": true, "databases": true, "certificates": true,
}

// p7 handles the Phase 7 collections; ok is false for anything else.
func (s *Server) p7(method, project string, parts []string, q map[string][]string, body map[string]any) (ok bool, status int, out any, e *apiErr) {
	if !p7Collections[parts[0]] {
		return false, 0, nil, nil
	}
	pp := "/projects/" + project
	switch parts[0] {
	case "storage":
		if method != http.MethodGet || len(parts) != 1 {
			return true, 0, nil, notFound("Route")
		}
		return true, 200, map[string]any{
			"project_id": project, "endpoint": s3Endpoint, "region": "neo",
			"buckets": len(s.sortedPaths(pp + "/buckets")), "access_keys": len(s.sortedPaths(pp + "/storage-keys")),
			"stored_bytes": 0, "quota_gib": 100, "state": "active",
		}, nil
	case "dns":
		if len(parts) < 2 || parts[1] != "zones" {
			return true, 0, nil, notFound("Route")
		}
		status, out, e := s.dns(method, project, parts[2:], body)
		return true, status, out, e
	}
	coll := parts[0]
	cp := pp + "/" + coll
	if len(parts) == 1 {
		switch method {
		case http.MethodGet:
			var items []any
			for _, p := range s.sortedPaths(cp) {
				if o, err := s.get(p); err == nil && matchQuery(o, q) {
					items = append(items, o)
				}
			}
			return true, 200, listPage(items), nil
		case http.MethodPost:
			status, out, e := s.createP7(project, coll, body)
			return true, status, out, e
		}
		return true, 0, nil, fail(405, "InvalidRequest", "Method not allowed")
	}
	if coll == "certificates" && parts[1] == "import-key" && method == http.MethodGet {
		return true, 200, map[string]any{
			"algorithm": "x25519-hkdf-sha256-aes256gcm", "public_key": ImportPublicKey, "format": "TBOX1",
		}, nil
	}
	if coll == "certificates" && parts[1] == "preflight" {
		return true, 200, map[string]any{"names": []any{}, "load_balancers": []any{}}, nil
	}
	ip := cp + "/" + parts[1]
	o, found := s.objs[ip]
	if !found {
		return true, 0, nil, notFound("Resource")
	}
	if len(parts) > 2 {
		if coll == "databases" {
			status, out, e := s.databasePart(method, o, parts[2:], body)
			return true, status, out, e
		}
		if coll != "load-balancers" {
			return true, 0, nil, notFound("Route")
		}
		status, out, e := s.lbPart(method, pp, ip, o, parts[2:], body)
		return true, status, out, e
	}
	switch method {
	case http.MethodGet:
		got, err := s.get(ip)
		if err != nil {
			return true, 0, nil, err
		}
		if coll == "storage-keys" {
			got = withoutSecret(got)
		}
		return true, 200, got, nil
	case http.MethodPatch:
		if o["state"] == "deleting" {
			return true, 0, nil, conflict("It is being deleted")
		}
		if err := s.patchP7(coll, o, body); err != nil {
			return true, 0, nil, err
		}
		o["state"] = "updating"
		s.settleLater(ip, false, nil)
		if coll == "storage-keys" {
			return true, 200, withoutSecret(o), nil
		}
		return true, 200, o, nil
	case http.MethodDelete:
		if coll == "buckets" && toFloat(o["object_count"]) > 0 && strings.Join(q["force"], "") != "true" {
			return true, 0, nil, fail(409, "Conflict", fmt.Sprintf(
				"The bucket holds %v object(s): delete them first, or delete the bucket with force", o["object_count"]))
		}
		o["state"] = "deleting"
		s.settleLater(ip, true, nil)
		return true, 202, o, nil
	}
	return true, 0, nil, fail(405, "InvalidRequest", "Method not allowed")
}

func withoutSecret(o map[string]any) map[string]any {
	c := clone(o)
	delete(c, "secret_access_key")
	delete(c, "region")
	return c
}

var p7Prefixes = map[string]string{
	"buckets": "bkt", "storage-keys": "s3key", "load-balancers": "lb", "snapshot-schedules": "ssch",
	"databases": "pgdb", "certificates": "cert",
}

var p7Services = map[string]string{
	"buckets": "storage", "storage-keys": "storage", "load-balancers": "network", "snapshot-schedules": "compute",
	"databases": "db", "certificates": "network",
}

func (s *Server) createP7(project, coll string, body map[string]any) (int, any, *apiErr) {
	pp := "/projects/" + project
	cp := pp + "/" + coll
	name := str(body["name"])
	if name == "" && coll == "certificates" {
		name = certName(body)
	}
	if name == "" {
		return 0, nil, fail(422, "InvalidRequest", "name: field required")
	}
	if s.nameTaken(cp, name) {
		return 0, nil, conflict(fmt.Sprintf("A resource named %q already exists", name))
	}
	id := s.id(p7Prefixes[coll])
	o := meta(id, project, name, p7Services[coll], body)
	regionZone(o, coll, body)
	var apply map[string]any
	switch coll {
	case "buckets":
		vis := str(body["visibility"])
		if vis == "" {
			vis = "private"
		}
		o["visibility"], o["max_size_gib"] = vis, body["max_size_gib"]
		o["stored_bytes"], o["object_count"] = 0, 0
		o["endpoint"], o["url"] = s3Endpoint, s3Endpoint+"/"+name
		o["observed_at"] = nil
	case "storage-keys":
		holder := "trn:tebib:iam::" + Org + ":user/u-test"
		if sa := str(body["service_account_id"]); sa != "" {
			obj := s.objs["/orgs/"+Org+"/service-accounts/"+sa]
			if obj == nil {
				return 0, nil, notFound("Service account " + sa)
			}
			holder = str(obj["trn"])
		}
		o["access_key_id"] = fmt.Sprintf("TBOMOCK%06d", s.seq)
		o["holder"], o["status"], o["effective_status"] = holder, "active", "pending"
		o["previous_access_key_id"], o["previous_expires_at"], o["rotated_at"] = nil, nil, nil
		o["endpoint"] = s3Endpoint
		apply = map[string]any{"effective_status": "active"}
		out := clone(o)
		out["secret_access_key"], out["region"] = "mock-secret-"+id, "neo"
		s.put(cp+"/"+id, o)
		s.settleLater(cp+"/"+id, false, apply)
		return 202, out, nil
	case "load-balancers":
		scheme := str(body["scheme"])
		if scheme == "" {
			scheme = "public"
		}
		subnetID := str(body["subnet_id"])
		if subnetID == "" {
			if def := s.objs[s.findDefault(pp+"/subnets")]; def != nil {
				subnetID = str(def["id"])
			}
		}
		subnet := s.objs[pp+"/subnets/"+subnetID]
		if subnet == nil {
			return 0, nil, fail(404, "NotFound", "Subnet "+subnetID+" not found")
		}
		sgs, _ := body["security_group_ids"].([]any)
		if len(sgs) == 0 {
			for _, p := range s.sortedPaths(pp + "/security-groups") {
				if s.objs[p]["vpc_id"] == subnet["vpc_id"] && s.objs[p]["is_default"] == true {
					sgs = []any{s.objs[p]["id"]}
				}
			}
		}
		o["scheme"], o["vpc_id"], o["subnet_id"], o["security_group_ids"] = scheme, subnet["vpc_id"], subnetID, sgs
		o["address"], o["public_ip_id"], o["public_address"] = "100.68.0.10", nil, nil
		if scheme == "public" {
			if ipID := str(body["public_ip_id"]); ipID != "" {
				ipObj := s.objs[pp+"/public-ips/"+ipID]
				if ipObj == nil {
					return 0, nil, fail(404, "NotFound", "Public IP "+ipID+" not found")
				}
				if ipObj["association"] != "none" {
					return 0, nil, conflict("The public IP is associated")
				}
				ipObj["association"] = "load-balancer"
				o["public_ip_id"], o["public_address"] = ipID, ipObj["address"]
			} else {
				o["public_address"] = fmt.Sprintf("203.0.113.%d", s.seq%250+1)
			}
		}
		o["nodes"] = map[string]any{"desired": 2, "ready": 0}
		o["listeners"], o["target_groups"] = []any{}, []any{}
		apply = map[string]any{"nodes": map[string]any{"desired": 2, "ready": 2}}
	case "databases":
		plan, mode := orDefault(body["plan"], DefaultDatabasePlan), orDefault(body["mode"], "single")
		spec, offered := databasePlans[str(plan)]
		if !offered {
			return 0, nil, fail(422, "InvalidRequest", fmt.Sprintf("plan: %q is not a database plan on offer", plan))
		}
		disk := body["disk_gib"]
		if disk == nil {
			disk = 10
		}
		backup, _ := body["backup"].(map[string]any)
		days := 7
		if v, ok := backup["retention_days"].(float64); ok {
			days = int(v)
		}
		version := body["version"]
		if version == nil {
			version = 18
		}
		o["version"], o["postgres_version"] = version, "18.6"
		o["plan"], o["mode"], o["vcpus"], o["memory_mib"] = plan, mode, spec.vcpus, spec.memoryMiB
		o["cpu_millicores"] = spec.cpuMilli
		o["disk_gib"], o["power_state"] = disk, "running"
		o["vpc_id"], o["subnet_id"] = "vpc-1", orDefault(body["subnet_id"], "subnet-1")
		o["address"] = "100.68.0.20"
		o["connection"] = map[string]any{
			"host": id + ".db.tebibcloud.com", "address": "100.68.0.20", "port": 5432, "user": "app",
			"database": "app", "sslmode": "verify-full", "ssl_negotiation": "classic",
		}
		o["ca_certificate"] = "-----BEGIN CERTIFICATE-----\nMOCK\n-----END CERTIFICATE-----\n"
		o["instances"] = map[string]any{"desired": 1, "running": 0, "ready": 0, "primary": nil, "standby": nil}
		o["backup"] = map[string]any{
			"enabled": backup["enabled"] == true, "retention_days": days, "status": "off",
			"last_success": nil, "recovery_window": nil, "size_bytes": 0,
		}
		o["maintenance_window"] = orDefault(body["maintenance_window"], "sun-02")
		o["public"] = databasePublic(false, nil, false, false)
		o["restore"], o["kept_backup_until"], o["disk_used_bytes"], o["events"] = nil, nil, nil, []any{}
		out := clone(o)
		out["password"] = "mock-password-" + id
		s.put(cp+"/"+id, o)
		s.settleLater(cp+"/"+id, false, map[string]any{
			"instances": map[string]any{"desired": 1, "running": 1, "ready": 1, "primary": id + "-1", "standby": nil},
		})
		return 202, out, nil
	case "certificates":
		imported := body["certificate"] != nil
		var names []any
		if imported {
			if !strings.HasPrefix(str(body["sealed_private_key"]), "VEJPWD") { // base64 of "TBOX..."
				return 0, nil, fail(400, "InvalidRequest", "sealed_private_key must be the private key sealed to GET .../import-key")
			}
			names = []any{"imported.example-site.dev"}
			o["source"], o["validation"] = "imported", ""
		} else {
			names, _ = body["domain_names"].([]any)
			if len(names) == 0 || str(body["validation"]) == "" {
				return 0, nil, fail(400, "InvalidRequest", "a managed certificate needs domain_names and validation")
			}
			o["source"], o["validation"] = "managed", body["validation"]
		}
		o["domain_names"] = names
		o["status"], o["status_code"], o["status_reason"] = "pending", "", ""
		o["not_before"], o["not_after"], o["renew_after"] = nil, nil, nil
		o["issuer"], o["fingerprint"], o["serial"] = "", "", ""
		o["test_certificate"], o["challenge_load_balancer_id"], o["used_by"] = false, nil, []any{}
		o["issuance"] = map[string]any{"status": "idle", "attempts": 0, "started_at": nil, "last_attempt_at": nil, "next_attempt_at": nil}
		s.put(cp+"/"+id, o)
		s.settleLater(cp+"/"+id, false, map[string]any{
			"status": "issued", "issuer": "Mock CA", "fingerprint": "ab12", "serial": "01",
			"not_before": "2030-01-01T00:00:00Z", "not_after": "2030-04-01T00:00:00Z", "renew_after": "2030-03-01T00:00:00Z",
		})
		return 202, o, nil
	case "snapshot-schedules":
		if s.objs[pp+"/vms/"+str(body["vm_id"])] == nil {
			return 0, nil, fail(404, "NotFound", "VM "+str(body["vm_id"])+" not found")
		}
		if body["retention_count"] == nil && body["retention_days"] == nil {
			return 0, nil, fail(400, "InvalidRequest", "retention_count: set retention_count, retention_days or both")
		}
		o["vm_id"] = body["vm_id"]
		if e := setTiming(o, body); e != nil {
			return 0, nil, e
		}
		o["retention_count"], o["retention_days"] = body["retention_count"], body["retention_days"]
		o["enabled"] = body["enabled"] != false
		o["next_run_at"], o["last_run"] = "2030-01-01T00:00:00Z", nil
		o["failures"], o["missed"], o["snapshot_count"] = 0, 0, 0
	}
	s.put(cp+"/"+id, o)
	s.settleLater(cp+"/"+id, false, apply)
	return 202, o, nil
}

// setTiming sets frequency, cron and timezone the way the API does (presets become cron).
func setTiming(o, body map[string]any) *apiErr {
	get := func(k string, def any) any {
		if v, ok := body[k]; ok && v != nil {
			return v
		}
		return def
	}
	freq := str(get("frequency", o["frequency"]))
	minute, hour, weekday := get("minute", 0), get("hour", 0), get("weekday", 0)
	cron := str(get("cron", ""))
	if freq == "custom" && cron == "" {
		cron = str(o["cron"])
	}
	switch freq {
	case "hourly":
		cron = fmt.Sprintf("%v * * * *", minute)
	case "daily":
		cron = fmt.Sprintf("%v %v * * *", minute, hour)
	case "weekly":
		cron = fmt.Sprintf("%v %v * * %v", minute, hour, weekday)
	case "custom":
		if cron == "" {
			return fail(400, "InvalidRequest", "cron: required when frequency is custom")
		}
	default:
		return fail(422, "InvalidRequest", "frequency: unknown")
	}
	if freq != "custom" && str(body["cron"]) != "" {
		return fail(400, "InvalidRequest", "cron: only with frequency custom")
	}
	o["frequency"], o["cron"] = freq, cron
	o["timezone"] = get("timezone", get("tz", o["timezone"]))
	if o["timezone"] == nil || o["timezone"] == "" {
		o["timezone"] = "UTC"
	}
	return nil
}

func (s *Server) patchP7(coll string, o, body map[string]any) *apiErr {
	for _, k := range []string{"description", "tags"} {
		if v, ok := body[k]; ok && v != nil {
			o[k] = v
		}
	}
	switch coll {
	case "buckets":
		if v := body["visibility"]; v != nil {
			o["visibility"] = v
		}
		if v := body["max_size_gib"]; v != nil {
			if toFloat(v) == 0 {
				o["max_size_gib"] = nil
			} else {
				o["max_size_gib"] = v
			}
		}
	case "storage-keys":
		if v := body["status"]; v != nil {
			o["status"], o["effective_status"] = v, v
		}
	case "databases":
		for _, k := range []string{"plan", "mode", "maintenance_window"} {
			if v := body[k]; v != nil {
				if k == "plan" {
					spec, offered := databasePlans[str(v)]
					if !offered {
						return fail(422, "InvalidRequest", fmt.Sprintf("plan: %q is not a database plan on offer", str(v)))
					}
					o["vcpus"], o["memory_mib"], o["cpu_millicores"] = spec.vcpus, spec.memoryMiB, spec.cpuMilli
				}
				o[k] = v
			}
		}
		if v := body["disk_gib"]; v != nil {
			if toFloat(v) < toFloat(o["disk_gib"]) {
				return fail(400, "InvalidRequest", "Disk can grow but never shrink; restore into a new database")
			}
			o["disk_gib"] = v
		}
		if b, ok := body["backup"].(map[string]any); ok {
			cur := o["backup"].(map[string]any)
			if v, ok := b["enabled"].(bool); ok {
				cur["enabled"] = v
			}
			if v := b["retention_days"]; v != nil {
				cur["retention_days"] = v
			}
		}
	case "snapshot-schedules":
		for _, k := range []string{"frequency", "minute", "hour", "weekday", "cron", "timezone"} {
			if body[k] != nil {
				if e := setTiming(o, body); e != nil {
					return e
				}
				break
			}
		}
		count, days := o["retention_count"], o["retention_days"]
		if body["clear_retention_count"] == true {
			count = nil
		}
		if body["clear_retention_days"] == true {
			days = nil
		}
		if body["retention_count"] != nil {
			count = body["retention_count"]
		}
		if body["retention_days"] != nil {
			days = body["retention_days"]
		}
		if count == nil && days == nil {
			return fail(400, "InvalidRequest", "retention_count: set retention_count, retention_days or both")
		}
		o["retention_count"], o["retention_days"] = count, days
		if v, ok := body["enabled"].(bool); ok {
			o["enabled"] = v
			if !v {
				o["next_run_at"] = nil
			} else {
				o["next_run_at"] = "2030-01-01T00:00:00Z"
			}
		}
	}
	return nil
}

// -- load balancer parts --------------------------------------------------------------------

func lbList(o map[string]any, key string) []any {
	l, _ := o[key].([]any)
	return l
}

func (s *Server) lbChanged(ip string, o map[string]any) (int, any, *apiErr) {
	o["state"] = "updating"
	s.settleLater(ip, false, nil)
	return 202, o, nil
}

func (s *Server) lbPart(method, pp, ip string, o map[string]any, rest []string, body map[string]any) (int, any, *apiErr) {
	if o["state"] == "deleting" {
		return 0, nil, conflict("The load balancer is being deleted")
	}
	switch {
	case rest[0] == "security-groups" && method == http.MethodPut && len(rest) == 1:
		o["security_group_ids"] = body["security_group_ids"]
		return s.lbChanged(ip, o)
	case rest[0] == "target-groups" && method == http.MethodPost && len(rest) == 1:
		for _, g := range lbList(o, "target_groups") {
			if g.(map[string]any)["name"] == body["name"] {
				return 0, nil, conflict(fmt.Sprintf("A target group named %q already exists", body["name"]))
			}
		}
		g := map[string]any{
			"id": s.id("lbtg"), "name": body["name"], "protocol": orDefault(body["protocol"], "tcp"),
			"port": body["port"], "algorithm": orDefault(body["algorithm"], "round_robin"),
			"proxy_protocol": body["proxy_protocol"] == true, "health_check": healthCheck(body["health_check"]),
			"verify_backend": body["verify_backend"] == true, "ca_bundle": orDefault(body["ca_bundle"], ""),
			"server_name": orDefault(body["server_name"], ""), "listener_ids": []any{},
			"targets": []any{}, "created_at": now(),
		}
		o["target_groups"] = append(lbList(o, "target_groups"), g)
		return s.lbChanged(ip, o)
	case rest[0] == "target-groups" && len(rest) >= 2:
		var g map[string]any
		idx := -1
		for i, x := range lbList(o, "target_groups") {
			if x.(map[string]any)["id"] == rest[1] {
				g, idx = x.(map[string]any), i
			}
		}
		if g == nil {
			return 0, nil, notFound("Target group")
		}
		switch {
		case len(rest) == 2 && method == http.MethodPatch:
			if name, ok := body["name"].(string); ok && name != g["name"] {
				for _, x := range lbList(o, "target_groups") {
					if x.(map[string]any)["name"] == name {
						return 0, nil, conflict(fmt.Sprintf("A target group named %q already exists", name))
					}
				}
			}
			for _, k := range []string{"name", "protocol", "port", "algorithm", "proxy_protocol", "verify_backend", "ca_bundle", "server_name"} {
				if v, ok := body[k]; ok && v != nil {
					g[k] = v
				}
			}
			if v := body["health_check"]; v != nil {
				g["health_check"] = healthCheck(v)
			}
			return s.lbChanged(ip, o)
		case len(rest) == 2 && method == http.MethodDelete:
			for _, l := range lbList(o, "listeners") {
				if l.(map[string]any)["target_group_id"] == rest[1] {
					return 0, nil, conflict("Listeners forward to this target group: " + str(l.(map[string]any)["id"]))
				}
			}
			groups := lbList(o, "target_groups")
			o["target_groups"] = append(append([]any{}, groups[:idx]...), groups[idx+1:]...)
			return s.lbChanged(ip, o)
		case len(rest) == 3 && rest[2] == "targets" && method == http.MethodPost:
			vm := s.objs[pp+"/vms/"+str(body["vm_id"])]
			if vm == nil {
				return 0, nil, fail(404, "NotFound", "VM "+str(body["vm_id"])+" not found")
			}
			for _, t := range lbList(g, "targets") {
				tm := t.(map[string]any)
				if tm["vm_id"] == body["vm_id"] && str(tm["port"]) == str(body["port"]) {
					return 0, nil, conflict("VM is already a target of the group on that port")
				}
			}
			port := body["port"]
			eff := port
			if eff == nil {
				eff = g["port"]
			}
			health := "unused"
			for _, l := range lbList(o, "listeners") {
				if l.(map[string]any)["target_group_id"] == g["id"] {
					health = "initial"
				}
			}
			g["targets"] = append(lbList(g, "targets"), map[string]any{
				"id": s.id("lbt"), "vm_id": body["vm_id"], "port": port, "effective_port": eff,
				"health": health, "health_reason": "", "created_at": now(),
			})
			return s.lbChanged(ip, o)
		case len(rest) == 4 && rest[2] == "targets" && method == http.MethodDelete:
			var keep []any
			found := false
			for _, t := range lbList(g, "targets") {
				if t.(map[string]any)["id"] == rest[3] {
					found = true
					continue
				}
				keep = append(keep, t)
			}
			if !found {
				return 0, nil, notFound("Target")
			}
			if keep == nil {
				keep = []any{}
			}
			g["targets"] = keep
			return s.lbChanged(ip, o)
		}
	case rest[0] == "listeners" && method == http.MethodPost && len(rest) == 1:
		for _, l := range lbList(o, "listeners") {
			if toFloat(l.(map[string]any)["port"]) == toFloat(body["port"]) {
				return 0, nil, conflict(fmt.Sprintf("Port %v already has a listener", body["port"]))
			}
		}
		redirect := body["redirect_to_https"] == true
		var group any = body["target_group_id"]
		if redirect {
			group = nil
		} else if !hasGroup(o, body["target_group_id"]) {
			return 0, nil, fail(404, "NotFound", "Target group "+str(body["target_group_id"])+" not found")
		}
		certs, _ := body["certificates"].([]any)
		var pending any
		if managed, ok := body["managed_certificate"].(map[string]any); ok && len(certs) == 0 {
			if names, _ := managed["domain_names"].([]any); len(names) == 0 {
				return 0, nil, fail(400, "InvalidRequest", "managed_certificate needs domain_names")
			}
			pending = s.id("cert") // issued later by the real API; the mock keeps it pending
		}
		if orDefault(body["protocol"], "tcp") == "https" && len(certs) == 0 && pending == nil {
			return 0, nil, fail(400, "InvalidRequest", "an https listener needs at least one certificate")
		}
		for _, c := range certs {
			cert := s.objs[pp+"/certificates/"+str(c)]
			if cert == nil {
				return 0, nil, notFound("Certificate " + str(c))
			}
			if cert["status"] != "issued" {
				return 0, nil, conflict("Certificate " + str(c) + " is " + str(cert["status"]) + ": only issued certificates can be attached")
			}
		}
		if certs == nil {
			certs = []any{}
		}
		o["listeners"] = append(lbList(o, "listeners"), map[string]any{
			"id": s.id("lbl"), "protocol": orDefault(body["protocol"], "tcp"), "port": body["port"],
			"target_group_id": group, "redirect_to_https": redirect,
			"redirect_port": redirectPort(body), "tls_policy": "tls12-intermediate",
			"certificate_ids": certs, "pending_certificate_id": pending,
			"status":     map[bool]string{true: "certificate_pending", false: "active"}[pending != nil],
			"created_at": now(),
		})
		return s.lbChanged(ip, o)
	case rest[0] == "listeners" && len(rest) == 2:
		listeners := lbList(o, "listeners")
		for i, x := range listeners {
			l := x.(map[string]any)
			if l["id"] != rest[1] {
				continue
			}
			switch method {
			case http.MethodPatch:
				if body["redirect_to_https"] == true {
					l["redirect_to_https"], l["target_group_id"] = true, nil
					if body["redirect_port"] != nil {
						l["redirect_port"] = body["redirect_port"]
					}
				} else if body["target_group_id"] != nil {
					if !hasGroup(o, body["target_group_id"]) {
						return 0, nil, fail(404, "NotFound", "Target group not found")
					}
					l["target_group_id"], l["redirect_to_https"] = body["target_group_id"], false
				}
				if certs, ok := body["certificates"].([]any); ok {
					if len(certs) == 0 {
						return 0, nil, conflict("An https listener can't be left without a certificate")
					}
					l["certificate_ids"] = certs
				}
				return s.lbChanged(ip, o)
			case http.MethodDelete:
				o["listeners"] = append(append([]any{}, listeners[:i]...), listeners[i+1:]...)
				return s.lbChanged(ip, o)
			}
		}
		return 0, nil, notFound("Listener")
	}
	return 0, nil, notFound("Route")
}

func hasGroup(o map[string]any, id any) bool {
	for _, g := range lbList(o, "target_groups") {
		if g.(map[string]any)["id"] == id {
			return true
		}
	}
	return false
}

func orDefault(v any, def string) any {
	if v == nil || v == "" {
		return def
	}
	return v
}

func healthCheck(v any) map[string]any {
	h := map[string]any{
		"protocol": "tcp", "port": nil, "path": "/", "matcher": "200-399",
		"interval_seconds": 10, "timeout_seconds": 5, "healthy_threshold": 3, "unhealthy_threshold": 3,
	}
	if in, ok := v.(map[string]any); ok {
		for k, x := range in {
			if x != nil {
				h[k] = x
			}
		}
	}
	return h
}

// -- DNS --------------------------------------------------------------------------------------

// canonicalValue mimics the API's presentation form: names fully qualified, TXT quoted.
func canonicalValue(rtype, v, domain string) string {
	qualify := func(name string) string {
		if strings.HasSuffix(name, ".") {
			return name
		}
		return name + "."
	}
	switch rtype {
	case "CNAME", "NS":
		return qualify(v)
	case "MX":
		f := strings.Fields(v)
		if len(f) == 2 {
			return f[0] + " " + qualify(f[1])
		}
	case "TXT":
		if !strings.HasPrefix(v, `"`) {
			return `"` + v + `"`
		}
	}
	return v
}

func (s *Server) dns(method, project string, parts []string, body map[string]any) (int, any, *apiErr) {
	pp := "/projects/" + project
	cp := pp + "/dns/zones"
	if len(parts) == 0 {
		switch method {
		case http.MethodGet:
			var items []any
			for _, p := range s.sortedPaths(cp) {
				if o, err := s.get(p); err == nil {
					items = append(items, o)
				}
			}
			return 200, listPage(items), nil
		case http.MethodPost:
			domain := strings.TrimSuffix(strings.ToLower(str(body["domain"])), ".")
			if domain == "" {
				return 0, nil, fail(422, "InvalidRequest", "domain: field required")
			}
			name := str(body["name"])
			if name == "" {
				name = domain
			}
			for _, p := range s.sortedPaths(cp) {
				if s.objs[p]["domain"] == domain {
					return 0, nil, conflict("The project already has a zone for " + domain)
				}
			}
			if s.nameTaken(cp, name) {
				return 0, nil, conflict(fmt.Sprintf("A zone named %q already exists", name))
			}
			id := s.id("zone")
			o := meta(id, project, name, "dns", body)
			o["domain"], o["status"], o["served"], o["serial"], o["delegation_ok"] = domain, "pending_verification", false, nil, nil
			o["nameservers"] = []any{"ns1.tebibcloud.com", "ns2.tebibcloud.com"}
			o["record_sets"] = 0
			o["verification"] = map[string]any{
				"state": "pending", "method": nil, "txt_name": "_tebib-verify." + domain,
				"txt_value": "tebib-verify=" + id, "message": "", "last_checked_at": nil, "next_check_at": nil,
			}
			// Like the API: "creating" until the domain is verified (VerifyZone).
			s.put(cp+"/"+id, o)
			return 202, o, nil
		}
		return 0, nil, fail(405, "InvalidRequest", "Method not allowed")
	}
	zp := cp + "/" + parts[0]
	zone, ok := s.objs[zp]
	if !ok {
		return 0, nil, notFound("Zone")
	}
	if len(parts) == 1 {
		switch method {
		case http.MethodPatch:
			if zone["state"] == "creating" {
				// A pending zone takes description and tags, and stays pending.
				for _, k := range []string{"description", "tags"} {
					if v, ok := body[k]; ok && v != nil {
						zone[k] = v
					}
				}
				return 200, zone, nil
			}
		case http.MethodDelete:
			for p := range s.objs {
				if strings.HasPrefix(p, zp+"/records/") {
					delete(s.objs, p)
				}
			}
		}
		return s.item(method, zp, body)
	}
	if parts[1] != "records" {
		return 0, nil, notFound("Route")
	}
	if zone["state"] == "deleting" {
		return 0, nil, conflict("The zone is being deleted")
	}
	domain := str(zone["domain"])
	rp := zp + "/records"
	if len(parts) == 2 {
		switch method {
		case http.MethodGet:
			var items []any
			for _, p := range s.sortedPaths(rp) {
				items = append(items, s.objs[p])
			}
			return 200, listPage(items), nil
		case http.MethodPost:
			name := strings.ToLower(strings.TrimSpace(str(orDefault(body["name"], "@"))))
			name = strings.TrimSuffix(strings.TrimSuffix(name, "."), "."+domain)
			if name == domain || name == "" {
				name = "@"
			}
			rtype := str(body["type"])
			for _, p := range s.sortedPaths(rp) {
				if s.objs[p]["name"] == name && s.objs[p]["type"] == rtype {
					return 0, nil, conflict(fmt.Sprintf("A %s record set named %s already exists", rtype, name))
				}
			}
			raw, _ := body["values"].([]any)
			var values []any
			for _, v := range raw {
				values = append(values, canonicalValue(rtype, str(v), domain))
			}
			fqdn := domain + "."
			if name != "@" {
				fqdn = name + "." + domain + "."
			}
			id := s.id("rec")
			ttl := body["ttl"]
			if ttl == nil {
				ttl = 3600
			}
			o := map[string]any{
				"id": id, "zone_id": zone["id"], "name": name, "fqdn": fqdn, "type": rtype, "ttl": ttl,
				"values": values, "description": str(body["description"]), "created_at": now(), "updated_at": now(),
			}
			s.put(rp+"/"+id, o)
			return 202, o, nil
		}
		return 0, nil, fail(405, "InvalidRequest", "Method not allowed")
	}
	p := rp + "/" + parts[2]
	rec, ok := s.objs[p]
	if !ok {
		return 0, nil, notFound("Record")
	}
	switch method {
	case http.MethodGet:
		return 200, rec, nil
	case http.MethodPatch:
		if v := body["ttl"]; v != nil {
			rec["ttl"] = v
		}
		if v := body["description"]; v != nil {
			rec["description"] = v
		}
		if raw, ok := body["values"].([]any); ok {
			var values []any
			for _, v := range raw {
				values = append(values, canonicalValue(str(rec["type"]), str(v), domain))
			}
			rec["values"] = values
		}
		return 202, rec, nil
	case http.MethodDelete:
		delete(s.objs, p)
		return 202, rec, nil
	}
	return 0, nil, fail(405, "InvalidRequest", "Method not allowed")
}

// -- managed databases -----------------------------------------------------------------------

func databasePublic(enabled bool, allow []any, allowAll, fallback bool) map[string]any {
	if allow == nil {
		allow = []any{}
	}
	out := map[string]any{
		"available": true, "enabled": enabled, "allow_all": allowAll, "allow_list": allow,
		"host": nil, "port": nil, "sslnegotiation": nil, "fallback_enabled": fallback, "fallback_port": nil,
	}
	if enabled {
		out["port"], out["sslnegotiation"] = 5432, "direct"
		if fallback {
			out["fallback_port"] = 15000
		}
	}
	return out
}

// databasePart handles /databases/{id}/exposure and the actions: the allow-list is required to
// open a database, like the API.
func (s *Server) databasePart(method string, o map[string]any, rest []string, body map[string]any) (int, any, *apiErr) {
	if method == http.MethodPut && rest[0] == "exposure" {
		public, _ := body["public"].(bool)
		allow, _ := body["allow_list"].([]any)
		allowAll, _ := body["allow_all"].(bool)
		fallback, _ := body["fallback"].(bool)
		if public && len(allow) == 0 && !allowAll {
			return 0, nil, fail(400, "InvalidRequest", "The public endpoint needs an allow_list (or allow_all)")
		}
		if fallback && !public {
			return 0, nil, fail(400, "InvalidRequest", "The fallback port belongs to the public endpoint: set public too")
		}
		pub := databasePublic(public, allow, allowAll, fallback)
		pub["host"] = nil
		if public {
			pub["host"] = str(o["id"]) + ".db.tebibcloud.com"
		}
		o["public"] = pub
		o["state"] = "updating"
		s.settleLater("/projects/"+str(o["project_id"])+"/databases/"+str(o["id"]), false, nil)
		return 202, o, nil
	}
	if method == http.MethodPost && (rest[0] == "start" || rest[0] == "stop" || rest[0] == "restart") {
		return 202, o, nil
	}
	return 0, nil, notFound("Route")
}

// ImportPublicKey is the X25519 public key the fake API hands out for sealing imported keys
// (base64 of 32 bytes; the tests never open the box).
const ImportPublicKey = "L1+yAecjr5HxOz1cEBBq9izKJ15W0AyHXaAswcU4ZHc="

func certName(body map[string]any) string {
	names, _ := body["domain_names"].([]any)
	if len(names) == 0 {
		return "imported-certificate"
	}
	return strings.NewReplacer(".", "-", "*", "wildcard").Replace(str(names[0]))
}

func redirectPort(body map[string]any) any {
	if body["redirect_port"] != nil {
		return body["redirect_port"]
	}
	return 443
}

// DefaultDatabasePlan is the plan a database gets when the request names none (the API's default).
const DefaultDatabasePlan = "b1.micro"

type dbPlanSpec struct {
	vcpus, cpuMilli, memoryMiB, diskGiB, maxDiskGiB, maxConn int
}

// databasePlans are the plans on offer, as the API's catalogue lists them (b1.large is hidden).
var databasePlans = map[string]dbPlanSpec{
	"b1.nano":   {1, 500, 512, 5, 50, 20},
	"b1.micro":  {1, 1000, 1024, 10, 100, 40},
	"b1.small":  {1, 1000, 2048, 10, 100, 50},
	"b1.medium": {2, 2000, 4096, 20, 250, 100},
}

var databasePlanOrder = []string{"b1.nano", "b1.micro", "b1.small", "b1.medium"}

func databasePlanList() []any {
	out := make([]any, 0, len(databasePlanOrder))
	for _, code := range databasePlanOrder {
		p := databasePlans[code]
		out = append(out, map[string]any{
			"code": code, "name": "db." + code, "vcpus": p.vcpus, "cpu_millicores": p.cpuMilli,
			"memory_mib": p.memoryMiB, "default_disk_gib": p.diskGiB, "max_disk_gib": p.maxDiskGiB,
			"max_connections": p.maxConn, "schedulable": true,
		})
	}
	return out
}

func databaseVersions() map[string]any {
	return map[string]any{
		"versions": []any{18, 17}, "default": 18, "ha_enabled": true,
		"maintenance_windows":        []any{"mon-02", "tue-02", "wed-02", "thu-02", "fri-02", "sat-02", "sun-02"},
		"default_maintenance_window": "sun-02",
	}
}
