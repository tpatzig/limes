INSERT INTO domains (id, cluster_id, name, uuid) VALUES (1, 'west', 'germany', 'uuid-for-germany');

INSERT INTO domain_services (id, domain_id, type) VALUES (1, 1, 'unittest');

INSERT INTO projects (id, domain_id, name, uuid, parent_uuid, has_bursting) VALUES (1, 1, 'berlin', 'uuid-for-berlin', 'uuid-for-germany', FALSE);
INSERT INTO projects (id, domain_id, name, uuid, parent_uuid, has_bursting) VALUES (2, 1, 'dresden', 'uuid-for-dresden', 'uuid-for-berlin', TRUE);

INSERT INTO project_services (id, project_id, type, scraped_at, stale) VALUES (1, 1, 'unittest', 5, FALSE);
INSERT INTO project_services (id, project_id, type, scraped_at, stale) VALUES (2, 2, 'unittest', 7, FALSE);

INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (1, 'capacity', 10, 0, 100, '', 10);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (1, 'things', 0, 2, 42, '[{"index":0},{"index":1}]', 0);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (2, 'capacity', 10, 0, 100, '', 12);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (2, 'things', 0, 2, 42, '[{"index":0},{"index":1}]', 0);
