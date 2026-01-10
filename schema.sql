-- Create listings table
CREATE TABLE IF NOT EXISTS listings (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    name VARCHAR(255),
    link TEXT,
    image TEXT,
    address TEXT,
    price NUMERIC(10, 2),
    area NUMERIC(10, 2),
    bedrooms INTEGER,
    type VARCHAR(100),
    for_sale BOOLEAN,
    parking INTEGER,
    content TEXT,
    photos JSONB,
    agency VARCHAR(255),
    bathrooms INTEGER,
    ref VARCHAR(255) UNIQUE,
    placeholder_image TEXT,
    agent_id INTEGER,
    published BOOLEAN DEFAULT false
);

-- Create index on ref for faster lookups
CREATE INDEX IF NOT EXISTS idx_listings_ref ON listings(ref);

-- Create index on published for filtering
CREATE INDEX IF NOT EXISTS idx_listings_published ON listings(published);
