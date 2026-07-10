-- Rename deployment profile canonical value: erebrus → standard (product label unchanged).
UPDATE nodes SET deployment_profile = 'standard' WHERE deployment_profile = 'erebrus';
UPDATE org_nodes SET deployment_profile = 'standard' WHERE deployment_profile = 'erebrus';
ALTER TABLE nodes ALTER COLUMN deployment_profile SET DEFAULT 'standard';