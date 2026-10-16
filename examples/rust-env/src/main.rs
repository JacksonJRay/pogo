use std::env;

fn main() {
    let keys = [
        "PORT",
        "HOST",
        "DATABASE_URL",
        "API_KEY",
        "JWT_SECRET",
        "QUOTED_VALUE",
        "SPACED_VALUE",
        "EQUALS_VALUE",
        "MULTILINE_VALUE",
        "VITE_PUBLIC_TITLE",
        "NEXT_PUBLIC_TITLE",
    ];
    let mut failed = false;
    for key in keys {
        match env::var(key) {
            Ok(value) => println!("POGO_ENV\t{key}\t{}", hex(value.as_bytes())),
            Err(_) => {
                println!("POGO_ENV\t{key}\tMISSING");
                failed = true;
            }
        }
    }
    if failed {
        std::process::exit(1);
    }
}

fn hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}
