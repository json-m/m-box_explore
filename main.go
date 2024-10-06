package main

import (
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"log"
	"math"
	"runtime"
	"strings"
)

const (
	width  = 1280
	height = 720
	title  = "3D Mandelbox Fractal Explorer"
)

var (
	vertexShaderSource = `
		#version 330 core
		layout (location = 0) in vec3 aPos;
		void main() {
			gl_Position = vec4(aPos.x, aPos.y, aPos.z, 1.0);
		}
	` + "\x00"

	fragmentShaderSource = `
		#version 330 core
		out vec4 FragColor;
		
		uniform vec3 cameraPos;
		uniform vec3 cameraFront;
		uniform vec3 cameraUp;
		uniform float scale;
		uniform int maxIterations;
		uniform vec2 resolution;
		uniform mat4 projection;

		uniform float debugZoom;
		uniform vec3 debugOffset;

		#define EPSILON 0.001
		#define MAX_DISTANCE 100.0
		#define MAX_STEPS 200

		float mandelboxDE(vec3 pos) {
			vec3 z = pos;
			float dr = 1.0;
			float r = 0.0;

			for (int i = 0; i < maxIterations; i++) {
				r = length(z);
				if (r > 6.0) break; // tweakable

				// Box fold
				z = clamp(z, -1.0, 1.0) * 2.0 - z;

				// Sphere fold
				if (r < 0.5) {
					z *= 4.0;
					dr *= 4.0;
				} else if (r < 1.0) {
					z /= r * r;
					dr /= r * r;
				}

				z = z * scale + pos;
				dr = dr * abs(scale) + 1.0;
			}

			return 0.5 * log(r) * r / dr;
		}

		vec3 hsv2rgb(vec3 c) {
			vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
			vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
			return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
		}

		void main() {
			vec2 uv = (gl_FragCoord.xy / resolution.xy) * 2.0 - 1.0;
			vec4 rayDir = projection * vec4(uv, -1.0, 1.0);
			rayDir = normalize(vec4(rayDir.xyz, 0.0));

			float t = 0.0;
			for (int i = 0; i < MAX_STEPS; i++) {
				vec3 p = cameraPos + t * rayDir.xyz;
				float d = mandelboxDE(p);
				if (d < EPSILON) {
					float hue = float(i) / 100.0;
					float sat = 0.8;
					float val = 1.0 - float(i) / 100.0;
					vec3 color = hsv2rgb(vec3(hue, sat, val));
					FragColor = vec4(color, 1.0);
					return;
				}
				t += d;
					if (t > MAX_DISTANCE) break;
			}
			FragColor = vec4(0.0, 0.0, 0.0, 1.0);
		}
	` + "\x00"
)

var (
	camera           mgl32.Vec3
	cameraFront      mgl32.Vec3
	cameraUp         mgl32.Vec3
	yaw              float32 = -90.0
	pitch            float32
	lastX            float64
	lastY            float64
	firstMouse       bool    = true
	scale            float32 = 2.0
	maxIterations    int32   = 100
	mouseSensitivity float32 = 0.05
	captureMouse     bool    = false
	projection       mgl32.Mat4
	debugZoom        float32 = 1.0
	debugOffset      mgl32.Vec3
)

func init() {
	runtime.LockOSThread()
}

func main() {
	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to initialize glfw:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(width, height, title, nil, nil)
	if err != nil {
		log.Fatalln("failed to create window:", err)
	}

	window.MakeContextCurrent()
	window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	window.SetCursorPosCallback(mouseMoveCallback)
	window.SetKeyCallback(keyCallback)

	if err := gl.Init(); err != nil {
		log.Fatalln("failed to initialize OpenGL:", err)
	}

	version := gl.GoStr(gl.GetString(gl.VERSION))
	fmt.Println("OpenGL version", version)

	program, vao := initOpenGL()

	initCamera()

	for !window.ShouldClose() {
		draw(window, program, vao)
	}
}

func initOpenGL() (uint32, uint32) {
	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		log.Fatalln("failed to compile vertex shader:", err)
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		log.Fatalln("failed to compile fragment shader:", err)
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		str := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(str))
		log.Fatalln("failed to link program:", str)
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	vertices := []float32{
		-1.0, -1.0, 0.0,
		1.0, -1.0, 0.0,
		-1.0, 1.0, 0.0,
		1.0, 1.0, 0.0,
	}

	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 3*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)

	return program, vao
}

func initCamera() {
	camera = mgl32.Vec3{0, 0, 0} // Move camera closer
	cameraFront = mgl32.Vec3{0, 0, -1}
	cameraUp = mgl32.Vec3{0, 1, 0}

	aspectRatio := float32(width) / float32(height)
	fov := float32(90.0) // FOV
	projection = mgl32.Perspective(mgl32.DegToRad(fov), aspectRatio, 0.1, 100.0)
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))
		return 0, fmt.Errorf("failed to compile shader: %v", log)
	}

	return shader, nil
}

func draw(window *glfw.Window, program uint32, vao uint32) {
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.UseProgram(program)

	cameraPosUniform := gl.GetUniformLocation(program, gl.Str("cameraPos\x00"))
	gl.Uniform3fv(cameraPosUniform, 1, &camera[0])

	cameraFrontUniform := gl.GetUniformLocation(program, gl.Str("cameraFront\x00"))
	gl.Uniform3fv(cameraFrontUniform, 1, &cameraFront[0])

	cameraUpUniform := gl.GetUniformLocation(program, gl.Str("cameraUp\x00"))
	gl.Uniform3fv(cameraUpUniform, 1, &cameraUp[0])

	scaleUniform := gl.GetUniformLocation(program, gl.Str("scale\x00"))
	gl.Uniform1f(scaleUniform, scale)

	maxIterationsUniform := gl.GetUniformLocation(program, gl.Str("maxIterations\x00"))
	gl.Uniform1i(maxIterationsUniform, maxIterations)

	resolutionUniform := gl.GetUniformLocation(program, gl.Str("resolution\x00"))
	gl.Uniform2f(resolutionUniform, float32(width), float32(height))

	projectionUniform := gl.GetUniformLocation(program, gl.Str("projection\x00"))
	gl.UniformMatrix4fv(projectionUniform, 1, false, &projection[0])

	gl.BindVertexArray(vao)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

	debugZoomUniform := gl.GetUniformLocation(program, gl.Str("debugZoom\x00"))
	gl.Uniform1f(debugZoomUniform, debugZoom)

	debugOffsetUniform := gl.GetUniformLocation(program, gl.Str("debugOffset\x00"))
	gl.Uniform3fv(debugOffsetUniform, 1, &debugOffset[0])

	window.SwapBuffers()
	glfw.PollEvents()
}

func mouseMoveCallback(window *glfw.Window, xpos float64, ypos float64) {
	if !captureMouse {
		return
	}

	if firstMouse {
		lastX = xpos
		lastY = ypos
		firstMouse = false
	}

	xoffset := xpos - lastX
	yoffset := lastY - ypos // Reversed since y-coordinates go from bottom to top
	lastX = xpos
	lastY = ypos

	xoffset *= float64(mouseSensitivity)
	yoffset *= float64(mouseSensitivity)

	yaw += float32(xoffset)
	pitch += float32(yoffset)

	if pitch > 89.0 {
		pitch = 89.0
	}
	if pitch < -89.0 {
		pitch = -89.0
	}

	front := mgl32.Vec3{
		float32(math.Cos(float64(mgl32.DegToRad(yaw))) * math.Cos(float64(mgl32.DegToRad(pitch)))),
		float32(math.Sin(float64(mgl32.DegToRad(pitch)))),
		float32(math.Sin(float64(mgl32.DegToRad(yaw))) * math.Cos(float64(mgl32.DegToRad(pitch)))),
	}
	cameraFront = front.Normalize()
}

func keyCallback(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	if action == glfw.Press {
		switch key {
		case glfw.KeyEscape:
			captureMouse = !captureMouse
			if captureMouse {
				window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
			} else {
				window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
			}
		case glfw.KeyLeft:
			mouseSensitivity -= 0.01
			if mouseSensitivity < 0.01 {
				mouseSensitivity = 0.01
			}
		case glfw.KeyRight:
			mouseSensitivity += 0.01
			if mouseSensitivity > 0.5 {
				mouseSensitivity = 0.5
			}
		}
	}

	if action == glfw.Press || action == glfw.Repeat {
		speed := float32(0.1)
		switch key {
		case glfw.KeyW:
			camera = camera.Add(cameraFront.Mul(speed))
		case glfw.KeyS:
			camera = camera.Sub(cameraFront.Mul(speed))
		case glfw.KeyA:
			camera = camera.Sub(cameraFront.Cross(cameraUp).Normalize().Mul(speed))
		case glfw.KeyD:
			camera = camera.Add(cameraFront.Cross(cameraUp).Normalize().Mul(speed))
		case glfw.KeyEqual:
			scale += 0.1
		case glfw.KeyMinus:
			scale -= 0.1
		case glfw.KeyQ:
			debugZoom *= 0.9
		case glfw.KeyE:
			debugZoom *= 1.1
		case glfw.KeyI:
			debugOffset[1] += 0.1
		case glfw.KeyK:
			debugOffset[1] -= 0.1
		case glfw.KeyJ:
			debugOffset[0] -= 0.1
		case glfw.KeyL:
			debugOffset[0] += 0.1
		case glfw.KeyU:
			debugOffset[2] -= 0.1
		case glfw.KeyO:
			debugOffset[2] += 0.1
		}
	}
}
