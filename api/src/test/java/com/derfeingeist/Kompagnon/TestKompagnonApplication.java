package com.derfeingeist.Kompagnon;

import org.springframework.boot.SpringApplication;

public class TestKompagnonApplication {

	public static void main(String[] args) {
		SpringApplication.from(KompagnonApplication::main).with(TestcontainersConfiguration.class).run(args);
	}

}
