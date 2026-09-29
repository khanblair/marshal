// The desktop entry point exists only so `cargo check` and IDEs treat this as an ordinary crate; a
// phone loads `run` through the mobile entry point in lib.rs.
fn main() {
    marshal_mobile_lib::run();
}
