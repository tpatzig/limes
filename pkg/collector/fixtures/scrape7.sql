INSERT INTO domains (id, cluster_id, name, uuid) VALUES (1, 'west', 'germany', 'uuid-for-germany');

INSERT INTO domain_services (id, domain_id, type) VALUES (1, 1, 'unittest');

INSERT INTO projects (id, domain_id, name, uuid, parent_uuid, has_bursting) VALUES (1, 1, 'berlin', 'uuid-for-berlin', 'uuid-for-germany', FALSE);
INSERT INTO projects (id, domain_id, name, uuid, parent_uuid, has_bursting) VALUES (2, 1, 'dresden', 'uuid-for-dresden', 'uuid-for-berlin', TRUE);

INSERT INTO project_services (id, project_id, type, scraped_at, stale) VALUES (1, 1, 'unittest', 26, FALSE);
INSERT INTO project_services (id, project_id, type, scraped_at, stale) VALUES (2, 2, 'unittest', 28, FALSE);

INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (1, 'capacity', 40, 0, 40, '', 40);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (1, 'things', 13, 5, 13, '[{"index":0},{"index":1},{"index":2},{"index":3},{"index":4}]', 13);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (2, 'capacity', 40, 0, 48, '', 48);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (2, 'things', 13, 5, 15, '[{"index":0},{"index":1},{"index":2},{"index":3},{"index":4}]', 15);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (1, 'external_things', 10, 0, 10, '', 10);
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources, desired_backend_quota) VALUES (2, 'external_things', 10, 0, 10, '', 10);
