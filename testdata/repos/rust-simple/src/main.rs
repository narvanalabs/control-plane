use actix_web::{get, web, App, HttpResponse, HttpServer, Responder};
use serde::Serialize;

#[derive(Serialize)]
struct HealthResponse {
    status: String,
}

#[derive(Serialize)]
struct MessageResponse {
    message: String,
}

#[get("/")]
async fn index() -> impl Responder {
    HttpResponse::Ok().json(MessageResponse {
        message: "Hello from Rust!".to_string(),
    })
}

#[get("/health")]
async fn health() -> impl Responder {
    HttpResponse::Ok().json(HealthResponse {
        status: "healthy".to_string(),
    })
}

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    let port = std::env::var("PORT").unwrap_or_else(|_| "8080".to_string());
    let addr = format!("0.0.0.0:{}", port);
    
    println!("Starting server at {}", addr);
    
    HttpServer::new(|| {
        App::new()
            .service(index)
            .service(health)
    })
    .bind(&addr)?
    .run()
    .await
}
