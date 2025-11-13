#!/usr/bin/env python3
"""
User generation script with batching and resume capability.

Features:
- Generates users in batches of 1000
- Saves to both CSV and database after each batch
- Can resume from interruption by checking existing data
- Progress tracking and ETA
"""

import bcrypt
from ulid import ULID
import time
import os
import sys
import json
from datetime import datetime

import sqlalchemy
from sqlalchemy import text
import numpy as np


# Database configuration
host = os.getenv("ISHOCON_DB_HOST", "127.0.0.1")
port = int(os.getenv("ISHOCON_DB_PORT", "3306"))
user = os.getenv("ISHOCON_DB_USER", "ishocon")
password = os.getenv("ISHOCON_DB_PASSWORD", "ishocon")
dbname = os.getenv("ISHOCON_DB_NAME", "ishocon3")

engine = sqlalchemy.create_engine(
    f"mysql+pymysql://{user}:{password}@{host}:{port}/{dbname}"
)

# Configuration
TOTAL_USER_COUNT = 50000
BATCH_SIZE = 1000
CSV_OUTPUT = 'users.csv'
PROGRESS_FILE = 'user_gen_progress.json'

# Credit amount distribution parameters (log-normal)
MU = 9.8        # Log-transformed mean
SIGMA = 0.915   # Log-transformed standard deviation
MIN_CREDIT = 5000
MAX_CREDIT = 300000


def generate_credit_amount(mu, sigma, min_val, max_val):
    """Generate credit amount using log-normal distribution"""
    while True:
        amount = np.random.lognormal(mean=mu, sigma=sigma)
        amount = int(round(amount))
        if min_val <= amount <= max_val:
            return amount


def load_progress():
    """Load progress from progress file"""
    if os.path.exists(PROGRESS_FILE):
        with open(PROGRESS_FILE, 'r') as f:
            return json.load(f)
    return {
        'last_completed_batch': -1,  # -1 means no batches completed
        'total_users_created': 0,
        'started_at': None,
        'last_updated': None
    }


def save_progress(progress):
    """Save progress to progress file"""
    progress['last_updated'] = datetime.now().isoformat()
    with open(PROGRESS_FILE, 'w') as f:
        json.dump(progress, f, indent=2)


def check_database_state():
    """Check how many users already exist in database"""
    with engine.begin() as conn:
        result = conn.execute(
            text("SELECT COUNT(*) FROM users WHERE name LIKE 'user%'")
        ).fetchone()
        return result[0]


def verify_csv_state():
    """Check how many users are in the CSV file"""
    if not os.path.exists(CSV_OUTPUT):
        return 0

    with open(CSV_OUTPUT, 'r') as f:
        # Subtract 1 for header row
        return sum(1 for _ in f) - 1


def create_users_batch(start_index, end_index, csv_file):
    """Create a batch of users and save to both CSV and database"""
    users_data = []

    # Generate user data
    for i in range(start_index, end_index):
        name = f"user{i + 1}"
        user_id = str(ULID())
        password = str(ULID())
        global_payment_token = str(ULID())
        encoded_salt = bcrypt.gensalt()
        salt = encoded_salt.decode()
        hashed_password = bcrypt.hashpw(password.encode(), encoded_salt).decode()
        credit_amount = generate_credit_amount(MU, SIGMA, MIN_CREDIT, MAX_CREDIT)

        users_data.append({
            'id': user_id,
            'name': name,
            'password': password,
            'hashed_password': hashed_password,
            'salt': salt,
            'global_payment_token': global_payment_token,
            'credit_amount': credit_amount
        })

    # Write to CSV
    for user in users_data:
        csv_file.write(f"{user['name']},{user['password']},{user['global_payment_token']},{user['credit_amount']}\n")
    csv_file.flush()

    # Insert into database
    with engine.begin() as conn:
        for user in users_data:
            conn.execute(
                text("""
                    INSERT INTO users (`id`, `name`, `hashed_password`, salt, is_admin, global_payment_token)
                    VALUES (:id, :name, :hashed_password, :salt, 0, :global_payment_token)
                """),
                {
                    'id': user['id'],
                    'name': user['name'],
                    'hashed_password': user['hashed_password'],
                    'salt': user['salt'],
                    'global_payment_token': user['global_payment_token']
                }
            )

    return len(users_data)


def print_progress(batch_num, total_batches, users_created, total_users, elapsed_time):
    """Print progress information"""
    pct = (users_created / total_users) * 100
    avg_time_per_batch = elapsed_time / (batch_num + 1)
    remaining_batches = total_batches - (batch_num + 1)
    eta_seconds = avg_time_per_batch * remaining_batches
    eta_minutes = eta_seconds / 60

    print(f"Batch {batch_num + 1}/{total_batches} completed | "
          f"Users: {users_created}/{total_users} ({pct:.1f}%) | "
          f"ETA: {eta_minutes:.1f}m")


def main():
    print("=" * 70)
    print("User Generation Script - Batched with Resume Support")
    print("=" * 70)
    print(f"Total users to generate: {TOTAL_USER_COUNT}")
    print(f"Batch size: {BATCH_SIZE}")
    print(f"Output CSV: {CSV_OUTPUT}")
    print()

    # Check current state
    progress = load_progress()
    db_user_count = check_database_state()
    csv_user_count = verify_csv_state()

    print("Current state:")
    print(f"  - Users in database: {db_user_count}")
    print(f"  - Users in CSV: {csv_user_count}")
    print(f"  - Last completed batch: {progress['last_completed_batch'] + 1}")
    print()

    # Determine starting point
    start_batch = progress['last_completed_batch'] + 1
    start_user_index = start_batch * BATCH_SIZE

    if start_user_index >= TOTAL_USER_COUNT:
        print("All users have already been generated!")
        return

    if start_batch > 0:
        print(f"Resuming from batch {start_batch + 1} (user {start_user_index + 1})")
        print()

        # Verify CSV consistency
        if csv_user_count != start_user_index:
            print(f"WARNING: CSV has {csv_user_count} users but expected {start_user_index}")
            response = input("Do you want to continue anyway? (yes/no): ")
            if response.lower() != 'yes':
                print("Aborted.")
                return

    # Initialize progress tracking
    if progress['started_at'] is None:
        progress['started_at'] = datetime.now().isoformat()

    start_time = time.time()
    total_batches = (TOTAL_USER_COUNT + BATCH_SIZE - 1) // BATCH_SIZE

    # Open CSV file in append mode if resuming, otherwise create new
    csv_mode = 'a' if start_batch > 0 else 'w'

    try:
        with open(CSV_OUTPUT, csv_mode) as csv_file:
            # Write header if starting fresh
            if start_batch == 0:
                csv_file.write('name,password,global_payment_token,credit_amount\n')

            # Process batches
            for batch_num in range(start_batch, total_batches):
                batch_start_index = batch_num * BATCH_SIZE
                batch_end_index = min((batch_num + 1) * BATCH_SIZE, TOTAL_USER_COUNT)

                print(f"Processing batch {batch_num + 1}/{total_batches} "
                      f"(users {batch_start_index + 1}-{batch_end_index})...", end=' ')

                batch_start_time = time.time()
                users_created = create_users_batch(batch_start_index, batch_end_index, csv_file)
                batch_elapsed = time.time() - batch_start_time

                print(f"✓ ({batch_elapsed:.1f}s)")

                # Update progress
                progress['last_completed_batch'] = batch_num
                progress['total_users_created'] = batch_end_index
                save_progress(progress)

                # Print overall progress
                elapsed = time.time() - start_time
                print_progress(batch_num, total_batches, batch_end_index, TOTAL_USER_COUNT, elapsed)
                print()

        print("=" * 70)
        print("✓ User generation completed successfully!")
        print(f"Total time: {(time.time() - start_time) / 60:.1f} minutes")
        print(f"CSV output: {CSV_OUTPUT}")
        print(f"Total users created: {TOTAL_USER_COUNT}")
        print("=" * 70)

        # Clean up progress file
        if os.path.exists(PROGRESS_FILE):
            os.remove(PROGRESS_FILE)
            print(f"Progress file removed: {PROGRESS_FILE}")

    except KeyboardInterrupt:
        print("\n\n" + "=" * 70)
        print("⚠ Interrupted by user")
        print(f"Progress saved. {progress['total_users_created']} users created so far.")
        print(f"To resume, run this script again.")
        print("=" * 70)
        sys.exit(1)
    except Exception as e:
        print(f"\n\nERROR: {e}")
        print(f"Progress saved. {progress['total_users_created']} users created so far.")
        print(f"To resume, run this script again.")
        sys.exit(1)


if __name__ == '__main__':
    main()
