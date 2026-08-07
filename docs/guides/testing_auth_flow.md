# Testing Authentication & Security Flows Locally

This guide provides step-by-step instructions for testing the end-to-end authentication flows in Odyssey ERP, including standard login, Single Sign-On (SSO), and 2-Factor Authentication (MFA/TOTP).

## Prerequisites

Ensure your local development environment is running:

1. **Start dependencies** (PostgreSQL, Redis, Mailpit, Gotenberg):
   ```bash
   docker compose up -d postgres redis mailpit gotenberg
   ```
2. **Apply migrations and seed the database**:
   ```bash
   PG_DSN=postgres://odyssey:odyssey@127.0.0.1:5432/odyssey?sslmode=disable make migrate-up
   PG_DSN=postgres://odyssey:odyssey@127.0.0.1:5432/odyssey?sslmode=disable make seed
   ```
3. **Run the Go application**:
   ```bash
   make air
   # OR
   go run cmd/odyssey/main.go
   ```

## 1. Testing Standard Login

1. Open your browser and navigate to `http://localhost:8080/auth/login`.
2. Enter the default administrator credentials:
   - **Email:** `admin@odyssey.test`
   - **Password:** `admin`
3. Click **Masuk**. You should be redirected to the main dashboard `/`.

## 2. Testing 2FA / MFA Setup (TOTP)

1. While logged in as `admin@odyssey.test`, click on your user profile picture in the top-right corner to open the dropdown menu.
2. Select **Setup MFA**.
3. You will be presented with a **Setup Key** (a secret code).
4. Use a TOTP authenticator application to generate a 6-digit code.
   - **Option A:** Use an app like Google Authenticator, Authy, or 1Password.
   - **Option B:** Use the command line `oathtool`:
     ```bash
     oathtool -b --totp "YOUR_SETUP_KEY"
     ```
5. Enter the generated 6-digit code into the confirmation box and click **Aktifkan MFA**.
6. You should see a success message indicating MFA is now active.

## 3. Testing 2FA / MFA Login Verification

1. After enabling MFA, open the user dropdown and click **Keluar** to log out.
2. Navigate back to `http://localhost:8080/auth/login`.
3. Log in again with `admin@odyssey.test` and `admin`.
4. The system will detect that MFA is enabled and intercept the login, redirecting you to `/auth/mfa/verify`.
5. Enter the current 6-digit TOTP code from your authenticator app (or `oathtool`).
6. Upon successful verification, you will be securely logged into the dashboard.

## 4. Testing Single Sign-On (SSO)

1. Ensure you are logged out of the application.
2. On the login page (`http://localhost:8080/auth/login`), click the **Single Sign-On (SSO)** button located below the primary login form.
3. The system will initiate an OIDC flow using the mock `ConnectionID: 1` loaded from the connectors module.
4. You will be redirected to the mock Identity Provider (IdP) for authentication.
5. Upon successful callback (`/auth/sso/callback`), the system will extract the email claim.
   - If the claim matches `admin@odyssey.test` and you enabled MFA earlier, you will be prompted for your TOTP code.
   - Otherwise, you will be immediately logged into the application.

## Troubleshooting

- **Database Connection Issues:** Ensure `PG_DSN` is correctly exported in your terminal session if you are running outside of Docker.
- **Invalid OTP Code:** TOTP is time-sensitive. Ensure your system clock is accurate.
- **SSO Mock Errors:** Verify that the mock OIDC connection is properly seeded in the `connector_connections` table.
