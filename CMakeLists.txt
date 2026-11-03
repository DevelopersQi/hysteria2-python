cmake_minimum_required(VERSION 3.12)
project(hysteria2 C CXX)

set(CMAKE_C_STANDARD 99)
set(CMAKE_CXX_STANDARD 11)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

if(NOT CMAKE_LIBRARY_OUTPUT_DIRECTORY)
    set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_SOURCE_DIR}/build/lib/${PROJECT_NAME})
endif()

set(HYSTERIA2_ARCHIVE "${CMAKE_SOURCE_DIR}/gobuild/hysteria2.a" CACHE FILEPATH "Go c-archive")
set(HYSTERIA2_INCLUDE_DIR "${CMAKE_SOURCE_DIR}/gobuild" CACHE PATH "Go c-archive headers")

add_subdirectory(pybind11)

pybind11_add_module(${PROJECT_NAME} src/hysteria2.cpp)

if(WIN32)
    if(MINGW)
        target_compile_options(${PROJECT_NAME} PRIVATE "-Wa,-mbig-obj")
        target_link_options(
            ${PROJECT_NAME} PRIVATE
            "-static-libgcc" "-static-libstdc++"
            "-Wl,-Bstatic,--whole-archive"
            "-lwinpthread"
            "-Wl,--no-whole-archive"
        )
    endif()
endif()

target_link_libraries(${PROJECT_NAME} PRIVATE "${HYSTERIA2_ARCHIVE}")
if(NOT WIN32)
    target_link_libraries(${PROJECT_NAME} PRIVATE "-lresolv")
    if(APPLE)
        target_link_libraries(${PROJECT_NAME} PRIVATE
            "-framework CoreFoundation"
            "-framework Security"
        )
    endif()
endif()

target_include_directories(${PROJECT_NAME} PRIVATE "${HYSTERIA2_INCLUDE_DIR}")
