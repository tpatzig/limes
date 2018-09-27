-- start data for inconsistencies test
-- "cloud" cluster has two domains
INSERT INTO domains (id, cluster_id, name, uuid) VALUES (1, 'cloud', 'germany',  'uuid-for-germany');
INSERT INTO domains (id, cluster_id, name, uuid) VALUES (2, 'cloud', 'pakistan', 'uuid-for-pakistan');

-- domain_services is fully populated (as ensured by the collector's consistency check)
INSERT INTO domain_services (id, domain_id, type) VALUES (1, 1, 'compute');
INSERT INTO domain_services (id, domain_id, type) VALUES (2, 1, 'network');
INSERT INTO domain_services (id, domain_id, type) VALUES (3, 2, 'compute');
INSERT INTO domain_services (id, domain_id, type) VALUES (4, 2, 'network');

-- domain_resources has a hole where no domain quota (pakistan: loadbalancers) has been set yet
INSERT INTO domain_resources (service_id, name, quota) VALUES (1, 'cores',         100);
INSERT INTO domain_resources (service_id, name, quota) VALUES (1, 'ram',           1000);
INSERT INTO domain_resources (service_id, name, quota) VALUES (2, 'loadbalancers', 20);
INSERT INTO domain_resources (service_id, name, quota) VALUES (3, 'cores',         30);
INSERT INTO domain_resources (service_id, name, quota) VALUES (3, 'ram',           250);

-- "germany" has one project, and "pakistan" has two (lahore is a child project of karachi in order to check
-- correct rendering of the parent_uuid field)
INSERT INTO projects (id, domain_id, name, uuid, parent_uuid) VALUES (1, 1, 'dresden', 'uuid-for-dresden', 'uuid-for-germany');
INSERT INTO projects (id, domain_id, name, uuid, parent_uuid) VALUES (2, 2, 'karachi', 'uuid-for-karachi', 'uuid-for-pakistan');
INSERT INTO projects (id, domain_id, name, uuid, parent_uuid) VALUES (3, 2, 'lahore',  'uuid-for-lahore',  'uuid-for-karachi');

-- project_services is fully populated (as ensured by the collector's consistency check)
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (1, 1, 'compute', '2018-06-13 15:06:37');
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (2, 1, 'network', '2018-06-13 15:06:37');
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (3, 2, 'compute', '2018-06-13 15:06:37');
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (4, 2, 'network', '2018-06-13 15:06:37');
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (5, 3, 'compute', '2018-06-13 15:06:37');
INSERT INTO project_services (id, project_id, type, scraped_at) VALUES (6, 3, 'network', '2018-06-13 15:06:37');

-- project_resources contains some pathological cases
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (1, 'cores',         30,  14, 10,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (1, 'ram',           100, 88, 100, '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (2, 'loadbalancers', 10,  5,  10,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (3, 'cores',         14,  18, 14,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (3, 'ram',           60,  45, 60,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (4, 'loadbalancers', 5,   2,  5,   '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (5, 'cores',         30,  20,  30,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (5, 'ram',           62,  48, 62,  '');
INSERT INTO project_resources (service_id, name, quota, usage, backend_quota, subresources) VALUES (6, 'loadbalancers', 10,  4,  10,  '');