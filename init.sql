-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create function to generate UUIDs (for compatibility)
CREATE OR REPLACE FUNCTION gen_random_uuid() RETURNS uuid AS $$
BEGIN
    RETURN uuid_generate_v4();
END;
$$ LANGUAGE plpgsql;

-- Insert default permissions after tables are created
-- This will be executed after the application creates the tables

-- Note: The actual permissions will be inserted by the application
-- This file can be used for any additional database setup